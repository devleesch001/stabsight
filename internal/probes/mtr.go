package probes

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"

	"github.com/devleesch001/stabsight/internal/config"
	"github.com/devleesch001/stabsight/internal/logging"
	"github.com/devleesch001/stabsight/internal/scheduler"
	"github.com/devleesch001/stabsight/internal/telemetry"
)

// MTRHop represents a single hop in a traceroute path.
type MTRHop struct {
	Hop     int           `json:"hop"`
	IP      string        `json:"ip"`
	RTT     time.Duration `json:"rtt"`
	Success bool          `json:"success"`
}

// MTRResult contains the complete traceroute path and reachability status.
type MTRResult struct {
	Target        string    `json:"target"`
	Hops          []MTRHop  `json:"hops"`
	ReachedTarget bool      `json:"reached_target"`
	Timestamp     time.Time `json:"timestamp"`
}

// MTRTracer abstracts the sequential TTL traceroute engine.
type MTRTracer interface {
	Trace(ctx context.Context, host string, maxHops int, timeout time.Duration) (*MTRResult, error)
}

// MTRProbe implements scheduler.ProbeWorker for MTR traceroutes.
type MTRProbe struct {
	name       string
	targetName string
	host       string
	maxHops    int
	interval   time.Duration
	timeout    time.Duration
	tracer     MTRTracer
	metrics    *telemetry.Metrics
	onResult   func(result *MTRResult)

	mu         sync.RWMutex
	lastResult *MTRResult
}

// NewMTRProbe instantiates a new MTRProbe worker.
func NewMTRProbe(
	targetName, host string,
	cfg *config.MTRProbeConfig,
	metrics *telemetry.Metrics,
	tracer MTRTracer,
	onResult func(result *MTRResult),
) *MTRProbe {
	interval := 60 * time.Second
	timeout := 1 * time.Second
	maxHops := 30

	if cfg != nil {
		if cfg.Interval > 0 {
			interval = cfg.Interval
		}
		if cfg.Timeout > 0 {
			timeout = cfg.Timeout
		}
		if cfg.MaxHops > 0 {
			maxHops = cfg.MaxHops
		}
	}

	if tracer == nil {
		tracer = &RealMTRTracer{}
	}

	return &MTRProbe{
		name:       fmt.Sprintf("%s/mtr", targetName),
		targetName: targetName,
		host:       host,
		maxHops:    maxHops,
		interval:   interval,
		timeout:    timeout,
		tracer:     tracer,
		metrics:    metrics,
		onResult:   onResult,
	}
}

// Name returns the probe worker identifier.
func (p *MTRProbe) Name() string { return p.name }

// TargetName returns the target host label.
func (p *MTRProbe) TargetName() string { return p.targetName }

// ProbeType returns "mtr".
func (p *MTRProbe) ProbeType() string { return "mtr" }

// IsExclusive returns false.
func (p *MTRProbe) IsExclusive() bool { return false }

// LastResult returns the most recent MTR traceroute result.
func (p *MTRProbe) LastResult() *MTRResult {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastResult
}

// Start executes the periodic MTR traceroute loop.
func (p *MTRProbe) Start(ctx context.Context, cmdChan <-chan scheduler.Command, ackChan chan<- struct{}) error {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// Initial immediate traceroute
	p.executeTrace(ctx)

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
			p.executeTrace(ctx)
		}
	}
}

// executeTrace performs the traceroute and updates telemetry and diagnostic callbacks.
func (p *MTRProbe) executeTrace(ctx context.Context) {
	traceCtx, cancel := context.WithTimeout(ctx, time.Duration(p.maxHops)*p.timeout)
	defer cancel()

	logger := logging.Get()
	logger.Debug().
		Str("probe", p.name).
		Str("target", p.targetName).
		Str("host", p.host).
		Int("max_hops", p.maxHops).
		Dur("timeout_per_hop", p.timeout).
		Msg("Starting MTR traceroute")

	start := time.Now()
	result, err := p.tracer.Trace(traceCtx, p.host, p.maxHops, p.timeout)
	if err != nil || result == nil {
		logger.Debug().
			Err(err).
			Str("probe", p.name).
			Str("target", p.targetName).
			Str("host", p.host).
			Int("max_hops", p.maxHops).
			Msg("MTR traceroute failed")
		return
	}

	result.Target = p.targetName
	result.Timestamp = time.Now()

	p.mu.Lock()
	p.lastResult = result
	p.mu.Unlock()

	logger.Debug().
		Str("probe", p.name).
		Str("target", p.targetName).
		Str("host", p.host).
		Int("hops_count", len(result.Hops)).
		Bool("reached_target", result.ReachedTarget).
		Dur("duration", time.Since(start)).
		Msg("MTR traceroute completed")

	if p.metrics != nil {
		for _, hop := range result.Hops {
			extraAttrs := []attribute.KeyValue{
				attribute.String("hop", strconv.Itoa(hop.Hop)),
				attribute.String("hop_ip", hop.IP),
			}
			if hop.Success {
				p.metrics.RecordRTT(ctx, hop.RTT.Seconds(), p.targetName, "mtr", "ipv4", extraAttrs...)
				p.metrics.RecordPacketLoss(ctx, 0.0, p.targetName, "mtr", "ipv4", extraAttrs...)
			} else {
				p.metrics.RecordPacketLoss(ctx, 1.0, p.targetName, "mtr", "ipv4", extraAttrs...)
			}
		}
	}

	if p.onResult != nil {
		p.onResult(result)
	}
}

// RealMTRTracer executes ICMP traceroute by sequentially incrementing TTL from 1 to maxHops.
type RealMTRTracer struct{}

// Trace performs sequential ICMP echo requests with increasing TTL.
func (t *RealMTRTracer) Trace(ctx context.Context, host string, maxHops int, timeout time.Duration) (*MTRResult, error) {
	ipAddr, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve host %q: %w", host, err)
	}

	isIPv6 := ipAddr.IP.To4() == nil
	result := &MTRResult{
		Target: host,
		Hops:   make([]MTRHop, 0, maxHops),
	}

	logger := logging.Get()

	for ttl := 1; ttl <= maxHops; ttl++ {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		hopCtx, cancel := context.WithTimeout(ctx, timeout)
		hop, reachedTarget, hopErr := t.pingHop(hopCtx, ipAddr, isIPv6, ttl, timeout)
		cancel()

		if hopErr != nil {
			logger.Debug().
				Str("host", host).
				Int("hop", ttl).
				Str("hop_ip", "*").
				Msg("MTR hop timeout")
			result.Hops = append(result.Hops, MTRHop{
				Hop:     ttl,
				IP:      "*",
				Success: false,
			})
		} else {
			logger.Debug().
				Str("host", host).
				Int("hop", ttl).
				Str("hop_ip", hop.IP).
				Dur("rtt", hop.RTT).
				Bool("reached_target", reachedTarget).
				Msg("MTR hop response")
			result.Hops = append(result.Hops, *hop)
			if reachedTarget {
				result.ReachedTarget = true
				break
			}
		}
	}

	return result, nil
}

var warnRawSocketOnce sync.Once

func logRawSocketWarning(err error) {
	warnRawSocketOnce.Do(func() {
		logger := logging.Get()
		logger.Warn().
			Err(err).
			Msg("MTR probe running in unprivileged UDP mode without CAP_NET_RAW. Intermediate hops will time out (hop_ip=*). To capture intermediate hops, run 'sudo setcap cap_net_raw+ep <binary>' or add '--cap-add=NET_RAW' in Docker.")
	})
}

// pingHop sends a single echo request with specified TTL and waits for TimeExceeded or EchoReply.
func (t *RealMTRTracer) pingHop(
	ctx context.Context,
	target *net.IPAddr,
	isIPv6 bool,
	ttl int,
	timeout time.Duration,
) (*MTRHop, bool, error) {
	var conn *icmp.PacketConn
	var pConn4 *ipv4.PacketConn
	var pConn6 *ipv6.PacketConn
	var icmpType icmp.Type
	var proto int
	var err error

	if isIPv6 {
		proto = 58
		icmpType = ipv6.ICMPTypeEchoRequest
		conn, err = icmp.ListenPacket("ip6:ipv6-icmp", "::")
		if err != nil {
			logRawSocketWarning(err)
			conn, err = icmp.ListenPacket("udp6", "::")
			if err != nil {
				return nil, false, err
			}
		}
		pConn6 = conn.IPv6PacketConn()
		if pConn6 != nil {
			_ = pConn6.SetHopLimit(ttl)
		}
	} else {
		proto = 1
		icmpType = ipv4.ICMPTypeEcho
		conn, err = icmp.ListenPacket("ip4:icmp", "0.0.0.0")
		if err != nil {
			logRawSocketWarning(err)
			conn, err = icmp.ListenPacket("udp4", "0.0.0.0")
			if err != nil {
				return nil, false, err
			}
		}
		pConn4 = conn.IPv4PacketConn()
		if pConn4 != nil {
			_ = pConn4.SetTTL(ttl)
		}
	}
	defer func() { _ = conn.Close() }()

	id := os.Getpid() & 0xffff
	seq := rand.Intn(0xffff) //nolint:gosec // random sequence number does not require cryptographic security
	msgBytes, err := (&icmp.Message{
		Type: icmpType,
		Code: 0,
		Body: &icmp.Echo{
			ID:   id,
			Seq:  seq,
			Data: []byte("stabsight-mtr"),
		},
	}).Marshal(nil)
	if err != nil {
		return nil, false, err
	}

	var dst net.Addr = target
	if strings.HasPrefix(conn.LocalAddr().Network(), "udp") {
		dst = &net.UDPAddr{IP: target.IP, Zone: target.Zone}
	}

	start := time.Now()
	_ = conn.SetDeadline(start.Add(timeout))

	if _, err := conn.WriteTo(msgBytes, dst); err != nil {
		return nil, false, err
	}

	buf := make([]byte, 1500)
	for {
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		default:
		}

		n, peer, err := conn.ReadFrom(buf)
		if err != nil {
			return nil, false, err
		}

		rtt := time.Since(start)
		parsedMsg, err := icmp.ParseMessage(proto, buf[:n])
		if err != nil {
			continue
		}

		peerIP := peer.String()
		if tcpOrUDP, _, splitErr := net.SplitHostPort(peerIP); splitErr == nil {
			peerIP = tcpOrUDP
		}

		switch parsedMsg.Type {
		case ipv4.ICMPTypeTimeExceeded, ipv6.ICMPTypeTimeExceeded:
			return &MTRHop{
				Hop:     ttl,
				IP:      peerIP,
				RTT:     rtt,
				Success: true,
			}, false, nil

		case ipv4.ICMPTypeEchoReply, ipv6.ICMPTypeEchoReply:
			return &MTRHop{
				Hop:     ttl,
				IP:      peerIP,
				RTT:     rtt,
				Success: true,
			}, true, nil
		}
	}
}
