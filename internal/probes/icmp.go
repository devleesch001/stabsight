package probes

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"

	"github.com/devleesch001/stabsight/internal/config"
	"github.com/devleesch001/stabsight/internal/logging"
	"github.com/devleesch001/stabsight/internal/scheduler"
	"github.com/devleesch001/stabsight/internal/telemetry"
)

// Pinger abstracts the low-level ICMP echo transmission.
type Pinger interface {
	Ping(ctx context.Context, host string, ipVersion string, timeout time.Duration) (time.Duration, error)
}

// ICMPProbe implements scheduler.ProbeWorker for ICMP ping measurements.
type ICMPProbe struct {
	name       string
	targetName string
	host       string
	ipVersion  string
	interval   time.Duration
	timeout    time.Duration
	count      int
	pinger     Pinger
	metrics    *telemetry.Metrics
	jitterCalc *JitterCalculator
}

// NewICMPProbe instantiates a new ICMPProbe worker.
func NewICMPProbe(targetName, host string, cfg *config.ICMPProbeConfig, ipVersion string, metrics *telemetry.Metrics, pinger Pinger) *ICMPProbe {
	interval := 1 * time.Second
	timeout := 1 * time.Second
	count := 1

	if cfg != nil {
		if cfg.Interval > 0 {
			interval = cfg.Interval
		}
		if cfg.Timeout > 0 {
			timeout = cfg.Timeout
		}
		if cfg.Count > 0 {
			count = cfg.Count
		}
	}

	if pinger == nil {
		pinger = &RealPinger{}
	}

	return &ICMPProbe{
		name:       fmt.Sprintf("%s/icmp", targetName),
		targetName: targetName,
		host:       host,
		ipVersion:  ipVersion,
		interval:   interval,
		timeout:    timeout,
		count:      count,
		pinger:     pinger,
		metrics:    metrics,
		jitterCalc: NewJitterCalculator(),
	}
}

// Name returns the probe unique identifier.
func (p *ICMPProbe) Name() string { return p.name }

// TargetName returns the target host label.
func (p *ICMPProbe) TargetName() string { return p.targetName }

// ProbeType returns "icmp".
func (p *ICMPProbe) ProbeType() string { return "icmp" }

// IsExclusive returns false.
func (p *ICMPProbe) IsExclusive() bool { return false }

// Start executes the periodic ICMP probe loop listening to scheduler control commands.
func (p *ICMPProbe) Start(ctx context.Context, cmdChan <-chan scheduler.Command, ackChan chan<- struct{}) error {
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
				// Send ACK on acknowledgement channel
				select {
				case ackChan <- struct{}{}:
				case <-ctx.Done():
					return ctx.Err()
				}

				// Wait in paused loop until CmdResume, CmdStop or ctx.Done()
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
				// Already active, continue
			}

		case <-ticker.C:
			p.executeCheck(ctx)
		}
	}
}

// executeCheck performs the ICMP ping and updates OTel metrics.
func (p *ICMPProbe) executeCheck(ctx context.Context) {
	pingCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	logger := logging.Get()
	rtt, err := p.pinger.Ping(pingCtx, p.host, p.ipVersion, p.timeout)
	if err != nil {
		logger.Debug().
			Err(err).
			Str("probe", p.name).
			Str("target", p.targetName).
			Str("host", p.host).
			Str("ip_version", p.ipVersion).
			Msg("ICMP ping failed (packet loss)")

		if p.metrics != nil {
			p.metrics.RecordPacketLoss(ctx, 1.0, p.targetName, "icmp", p.ipVersion)
		}
		return
	}

	rttSeconds := rtt.Seconds()
	jitterVal := 0.0
	var hasJitter bool

	// FR5: Real-time Jitter calculation = |RTT_n - RTT_(n-1)| without smoothing
	if jitter, ok := p.jitterCalc.Compute(rttSeconds); ok {
		jitterVal = jitter
		hasJitter = true
		if p.metrics != nil {
			p.metrics.RecordJitter(ctx, jitter, p.targetName, "icmp", p.ipVersion)
		}
	}

	logEvent := logger.Debug().
		Str("probe", p.name).
		Str("target", p.targetName).
		Str("host", p.host).
		Str("ip_version", p.ipVersion).
		Dur("rtt", rtt)

	if hasJitter {
		logEvent = logEvent.Float64("jitter_seconds", jitterVal)
	}
	logEvent.Msg("ICMP ping success")

	if p.metrics != nil {
		p.metrics.RecordRTT(ctx, rttSeconds, p.targetName, "icmp", p.ipVersion)
		p.metrics.RecordPacketLoss(ctx, 0.0, p.targetName, "icmp", p.ipVersion)
	}
}

// RealPinger performs actual ICMP echo requests using RAW or unprivileged UDP sockets.
type RealPinger struct{}

// Ping sends an ICMP Echo request and measures the round-trip time.
func (rp *RealPinger) Ping(ctx context.Context, host string, ipVersion string, timeout time.Duration) (time.Duration, error) {
	// Resolve IP address
	network := "ip"
	switch ipVersion {
	case "ipv4":
		network = "ip4"
	case "ipv6":
		network = "ip6"
	}

	ipAddr, err := net.ResolveIPAddr(network, host)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve host %q: %w", host, err)
	}

	isIPv6 := ipAddr.IP.To4() == nil

	var conn *icmp.PacketConn
	var icmpType icmp.Type
	var proto int

	if isIPv6 {
		proto = 58 // IPv6-ICMP
		icmpType = ipv6.ICMPTypeEchoRequest
		// Try privileged socket first, fallback to UDP unprivileged
		conn, err = icmp.ListenPacket("ip6:ipv6-icmp", "::")
		if err != nil {
			conn, err = icmp.ListenPacket("udp6", "::")
			if err != nil {
				return 0, fmt.Errorf("failed to open ICMPv6 connection: %w", err)
			}
		}
	} else {
		proto = 1 // ICMP
		icmpType = ipv4.ICMPTypeEcho
		// Try privileged raw socket first, fallback to UDP unprivileged
		conn, err = icmp.ListenPacket("ip4:icmp", "0.0.0.0")
		if err != nil {
			conn, err = icmp.ListenPacket("udp4", "0.0.0.0")
			if err != nil {
				return 0, fmt.Errorf("failed to open ICMP connection: %w", err)
			}
		}
	}
	defer func() { _ = conn.Close() }()

	id := rand.Intn(0xffff)  //nolint:gosec // random ID does not require cryptographic security
	seq := rand.Intn(0xffff) //nolint:gosec // random ICMP sequence number does not require cryptographic security
	msgBytes, err := (&icmp.Message{
		Type: icmpType,
		Code: 0,
		Body: &icmp.Echo{
			ID:   id,
			Seq:  seq,
			Data: []byte("stabsight-ping"),
		},
	}).Marshal(nil)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal ICMP packet: %w", err)
	}

	var dst net.Addr = ipAddr
	// In unprivileged UDP mode, net.UDPAddr is required
	if strings.HasPrefix(conn.LocalAddr().Network(), "udp") {
		dst = &net.UDPAddr{IP: ipAddr.IP, Zone: ipAddr.Zone}
	}

	start := time.Now()
	deadline := start.Add(timeout)
	if err := conn.SetDeadline(deadline); err != nil {
		return 0, fmt.Errorf("failed to set deadline: %w", err)
	}

	if _, err := conn.WriteTo(msgBytes, dst); err != nil {
		return 0, fmt.Errorf("failed to send ICMP packet: %w", err)
	}

	replyBuf := make([]byte, 1500)
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		n, _, err := conn.ReadFrom(replyBuf)
		if err != nil {
			return 0, fmt.Errorf("failed to read ICMP reply: %w", err)
		}

		rtt := time.Since(start)

		parsedMsg, err := icmp.ParseMessage(proto, replyBuf[:n])
		if err != nil {
			continue
		}

		switch parsedMsg.Type {
		case ipv4.ICMPTypeEchoReply, ipv6.ICMPTypeEchoReply:
			if echo, ok := parsedMsg.Body.(*icmp.Echo); ok {
				if echo.Seq == seq || echo.ID == id {
					return rtt, nil
				}
			}
		}
	}
}
