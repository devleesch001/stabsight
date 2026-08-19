package probes

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/devleesch001/stabsight/internal/config"
	"github.com/devleesch001/stabsight/internal/scheduler"
	"github.com/devleesch001/stabsight/internal/telemetry"
)

// TCPDialer abstracts TCP connection establishment for testability.
type TCPDialer interface {
	Dial(ctx context.Context, network, address string, timeout time.Duration) (time.Duration, error)
}

// TCPProbe implements scheduler.ProbeWorker for TCP connection latency checks.
type TCPProbe struct {
	name       string
	targetName string
	host       string
	port       int
	ipVersion  string
	interval   time.Duration
	timeout    time.Duration
	dialer     TCPDialer
	metrics    *telemetry.Metrics
	jitterCalc *JitterCalculator
}

// NewTCPProbe instantiates a new TCPProbe worker.
func NewTCPProbe(targetName, host string, cfg *config.TCPProbeConfig, ipVersion string, metrics *telemetry.Metrics, dialer TCPDialer) *TCPProbe {
	interval := 5 * time.Second
	timeout := 2 * time.Second
	port := 80

	if cfg != nil {
		if cfg.Interval > 0 {
			interval = cfg.Interval
		}
		if cfg.Timeout > 0 {
			timeout = cfg.Timeout
		}
		if cfg.Port > 0 {
			port = cfg.Port
		}
	}

	if dialer == nil {
		dialer = &RealTCPDialer{}
	}

	return &TCPProbe{
		name:       fmt.Sprintf("%s/tcp", targetName),
		targetName: targetName,
		host:       host,
		port:       port,
		ipVersion:  ipVersion,
		interval:   interval,
		timeout:    timeout,
		dialer:     dialer,
		metrics:    metrics,
		jitterCalc: NewJitterCalculator(),
	}
}

// Name returns the probe worker identifier.
func (p *TCPProbe) Name() string { return p.name }

// TargetName returns the target name.
func (p *TCPProbe) TargetName() string { return p.targetName }

// ProbeType returns "tcp".
func (p *TCPProbe) ProbeType() string { return "tcp" }

// IsExclusive returns false.
func (p *TCPProbe) IsExclusive() bool { return false }

// Start executes the periodic TCP probe worker loop.
func (p *TCPProbe) Start(ctx context.Context, cmdChan <-chan scheduler.Command, ackChan chan<- struct{}) error {
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
				select {
				case ackChan <- struct{}{}:
				case <-ctx.Done():
					return ctx.Err()
				}

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
			}

		case <-ticker.C:
			p.executeCheck(ctx)
		}
	}
}

// executeCheck performs the TCP connect check and reports metrics to OTel.
func (p *TCPProbe) executeCheck(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	network := "tcp"
	switch p.ipVersion {
	case "ipv4":
		network = "tcp4"
	case "ipv6":
		network = "tcp6"
	}

	address := net.JoinHostPort(p.host, strconv.Itoa(p.port))
	rtt, err := p.dialer.Dial(checkCtx, network, address, p.timeout)

	extraAttrs := []attribute.KeyValue{
		attribute.Int("port", p.port),
	}

	if err != nil {
		if p.metrics != nil {
			p.metrics.RecordPacketLoss(ctx, 1.0, p.targetName, "tcp", p.ipVersion, extraAttrs...)
		}
		return
	}

	rttSeconds := rtt.Seconds()

	if jitter, ok := p.jitterCalc.Compute(rttSeconds); ok {
		if p.metrics != nil {
			p.metrics.RecordJitter(ctx, jitter, p.targetName, "tcp", p.ipVersion, extraAttrs...)
		}
	}

	if p.metrics != nil {
		p.metrics.RecordRTT(ctx, rttSeconds, p.targetName, "tcp", p.ipVersion, extraAttrs...)
		p.metrics.RecordPacketLoss(ctx, 0.0, p.targetName, "tcp", p.ipVersion, extraAttrs...)
	}
}

// RealTCPDialer performs genuine TCP handshakes using net.Dialer.
type RealTCPDialer struct{}

// Dial establishes a TCP connection, measures duration, and terminates the connection.
func (d *RealTCPDialer) Dial(ctx context.Context, network, address string, timeout time.Duration) (time.Duration, error) {
	dialer := &net.Dialer{
		Timeout: timeout,
	}

	start := time.Now()
	conn, err := dialer.DialContext(ctx, network, address)
	elapsed := time.Since(start)

	if err != nil {
		return 0, fmt.Errorf("TCP dial failed: %w", err)
	}

	_ = conn.Close()
	return elapsed, nil
}
