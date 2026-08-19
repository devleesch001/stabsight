package probes

import (
	"context"
	"fmt"
	"math"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"go.opentelemetry.io/otel/attribute"

	"github.com/devleesch001/stabsight/internal/config"
	"github.com/devleesch001/stabsight/internal/scheduler"
	"github.com/devleesch001/stabsight/internal/telemetry"
)

// DNSResolver abstracts DNS resolution queries.
type DNSResolver interface {
	Resolve(ctx context.Context, host string, recordType string, server string, timeout time.Duration) (time.Duration, error)
}

// DNSProbe implements scheduler.ProbeWorker for DNS resolution latency measurements.
type DNSProbe struct {
	name       string
	targetName string
	host       string
	ipVersion  string
	interval   time.Duration
	timeout    time.Duration
	server     string
	recordType string
	resolver   DNSResolver
	metrics    *telemetry.Metrics

	mu      sync.Mutex
	lastRTT float64
	hasRTT  bool
}

// NewDNSProbe instantiates a new DNSProbe worker.
func NewDNSProbe(targetName, host string, cfg *config.DNSProbeConfig, ipVersion string, metrics *telemetry.Metrics, resolver DNSResolver) *DNSProbe {
	interval := 5 * time.Second
	timeout := 2 * time.Second
	server := "8.8.8.8:53"
	recordType := "A"

	if cfg != nil {
		if cfg.Interval > 0 {
			interval = cfg.Interval
		}
		if cfg.Timeout > 0 {
			timeout = cfg.Timeout
		}
		if cfg.Server != "" {
			server = cfg.Server
		}
		if cfg.RecordType != "" {
			recordType = strings.ToUpper(cfg.RecordType)
		}
	}

	if resolver == nil {
		resolver = &RealDNSResolver{}
	}

	return &DNSProbe{
		name:       fmt.Sprintf("%s/dns", targetName),
		targetName: targetName,
		host:       host,
		ipVersion:  ipVersion,
		interval:   interval,
		timeout:    timeout,
		server:     server,
		recordType: recordType,
		resolver:   resolver,
		metrics:    metrics,
	}
}

// Name returns the probe worker identifier.
func (p *DNSProbe) Name() string { return p.name }

// TargetName returns the target name.
func (p *DNSProbe) TargetName() string { return p.targetName }

// ProbeType returns "dns".
func (p *DNSProbe) ProbeType() string { return "dns" }

// IsExclusive returns false.
func (p *DNSProbe) IsExclusive() bool { return false }

// Start executes the periodic DNS probe loop and responds to scheduler control signals.
func (p *DNSProbe) Start(ctx context.Context, cmdChan <-chan scheduler.Command, ackChan chan<- struct{}) error {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// Initial immediate measurement
	p.executeCheck(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case cmd := <-cmdChan:
			switch cmd {
			case scheduler.CmdPause:
				// Acknowledge pause
				select {
				case ackChan <- struct{}{}:
				case <-ctx.Done():
					return ctx.Err()
				}

				// Wait in pause loop
			pauseLoop:
				for {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case resumeCmd := <-cmdChan:
						if resumeCmd == scheduler.CmdResume {
							break pauseLoop
						}
						if resumeCmd == scheduler.CmdStop {
							return nil
						}
					}
				}

			case scheduler.CmdStop:
				return nil
			case scheduler.CmdResume:
				// Already active
			}

		case <-ticker.C:
			p.executeCheck(ctx)
		}
	}
}

// executeCheck performs the DNS query and records OTel metrics.
func (p *DNSProbe) executeCheck(ctx context.Context) {
	resolveCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	rtt, err := p.resolver.Resolve(resolveCtx, p.host, p.recordType, p.server, p.timeout)
	extraAttrs := []attribute.KeyValue{
		attribute.String("record_type", p.recordType),
	}

	if err != nil {
		if p.metrics != nil {
			p.metrics.RecordPacketLoss(ctx, 1.0, p.targetName, "dns", p.ipVersion, extraAttrs...)
		}
		return
	}

	rttSeconds := rtt.Seconds()

	p.mu.Lock()
	if p.hasRTT {
		// FR5: Real-time Jitter calculation without smoothing
		jitter := math.Abs(rttSeconds - p.lastRTT)
		if p.metrics != nil {
			p.metrics.RecordJitter(ctx, jitter, p.targetName, "dns", p.ipVersion, extraAttrs...)
		}
	}
	p.lastRTT = rttSeconds
	p.hasRTT = true
	p.mu.Unlock()

	if p.metrics != nil {
		p.metrics.RecordRTT(ctx, rttSeconds, p.targetName, "dns", p.ipVersion, extraAttrs...)
		p.metrics.RecordPacketLoss(ctx, 0.0, p.targetName, "dns", p.ipVersion, extraAttrs...)
	}
}

// RealDNSResolver sends DNS queries via github.com/miekg/dns.
type RealDNSResolver struct{}

// Resolve executes a DNS query and returns the response round-trip time.
func (r *RealDNSResolver) Resolve(ctx context.Context, host string, recordType string, server string, timeout time.Duration) (time.Duration, error) {
	if !strings.Contains(server, ":") {
		server = net.JoinHostPort(server, "53")
	}

	qType := dns.TypeA
	switch strings.ToUpper(recordType) {
	case "AAAA":
		qType = dns.TypeAAAA
	case "CNAME":
		qType = dns.TypeCNAME
	case "TXT":
		qType = dns.TypeTXT
	case "MX":
		qType = dns.TypeMX
	case "NS":
		qType = dns.TypeNS
	case "PTR":
		qType = dns.TypePTR
	case "SOA":
		qType = dns.TypeSOA
	}

	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(host), qType)
	msg.RecursionDesired = true

	client := &dns.Client{
		Timeout: timeout,
	}

	start := time.Now()
	resp, rtt, err := client.ExchangeContext(ctx, msg, server)
	if err != nil {
		return 0, fmt.Errorf("DNS query failed: %w", err)
	}

	if resp == nil {
		return 0, fmt.Errorf("empty DNS response from %s", server)
	}

	if resp.Rcode != dns.RcodeSuccess && resp.Rcode != dns.RcodeNameError {
		return 0, fmt.Errorf("DNS server returned error code: %s", dns.RcodeToString[resp.Rcode])
	}

	if rtt == 0 {
		rtt = time.Since(start)
	}

	return rtt, nil
}
