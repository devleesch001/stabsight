package diagnostic

import (
	"time"

	"github.com/rs/zerolog"

	"github.com/devleesch001/stabsight/internal/probes"
)

// DiagnosisType represents the classified root cause of network degradation.
type DiagnosisType string

// Diagnosis classification constants.
const (
	DiagnosisOK                DiagnosisType = "normal"
	DiagnosisLocalNetwork      DiagnosisType = "local_network_issue"
	DiagnosisISPGateway        DiagnosisType = "isp_gateway_loss"
	DiagnosisTransitLoss       DiagnosisType = "transit_route_loss"
	DiagnosisTargetUnreachable DiagnosisType = "target_unreachable"
	DiagnosisHighLatency       DiagnosisType = "high_latency_detected"
)

// Report contains diagnostic correlation details.
type Report struct {
	Target        string        `json:"target"`
	Diagnosis     DiagnosisType `json:"diagnosis"`
	LossRatio     float64       `json:"loss_ratio"`
	RTT           time.Duration `json:"rtt"`
	FaultyHop     int           `json:"faulty_hop,omitempty"`
	FaultyHopIP   string        `json:"faulty_hop_ip,omitempty"`
	Message       string        `json:"message"`
	Timestamp     time.Time     `json:"timestamp"`
}

// Engine correlates probe anomalies with MTR traceroutes and emits Zerolog structured logs.
type Engine struct {
	logger           zerolog.Logger
	lossThreshold    float64
	latencyThreshold time.Duration
}

// NewEngine instantiates a new Diagnostic Engine.
func NewEngine(logger zerolog.Logger, lossThreshold float64, latencyThreshold time.Duration) *Engine {
	if lossThreshold <= 0 {
		lossThreshold = 0.05 // default 5% packet loss
	}
	if latencyThreshold <= 0 {
		latencyThreshold = 250 * time.Millisecond // default 250ms
	}

	return &Engine{
		logger:           logger,
		lossThreshold:    lossThreshold,
		latencyThreshold: latencyThreshold,
	}
}

// Analyze correlates target metrics with MTR traceroute data and logs structured findings.
func (e *Engine) Analyze(target string, lossRatio float64, rtt time.Duration, mtr *probes.MTRResult) *Report {
	hasLossAnomaly := lossRatio >= e.lossThreshold
	hasLatencyAnomaly := rtt >= e.latencyThreshold

	if !hasLossAnomaly && !hasLatencyAnomaly {
		return &Report{
			Target:    target,
			Diagnosis: DiagnosisOK,
			LossRatio: lossRatio,
			RTT:       rtt,
			Message:   "network connectivity operating normally",
			Timestamp: time.Now(),
		}
	}

	diagnosis := DiagnosisTransitLoss
	faultyHop := 0
	faultyHopIP := ""
	message := "network degradation detected"

	if mtr != nil && len(mtr.Hops) > 0 {
		for _, hop := range mtr.Hops {
			if !hop.Success || (hop.RTT > e.latencyThreshold && hop.RTT > 0) {
				faultyHop = hop.Hop
				faultyHopIP = hop.IP

				switch {
				case hop.Hop == 1:
					diagnosis = DiagnosisLocalNetwork
					message = "packet loss or high latency on local gateway / router"
				case hop.Hop <= 3:
					diagnosis = DiagnosisISPGateway
					message = "packet loss or high latency on ISP gateway or first access node"
				default:
					diagnosis = DiagnosisTransitLoss
					message = "packet loss or latency spike on intermediate transit network"
				}
				break
			}
		}

		if faultyHop == 0 && !mtr.ReachedTarget {
			diagnosis = DiagnosisTargetUnreachable
			message = "destination target host unreachable"
		}
	} else if hasLossAnomaly && lossRatio >= 1.0 {
		diagnosis = DiagnosisTargetUnreachable
		message = "target unreachable (100% packet loss)"
	} else if hasLatencyAnomaly {
		diagnosis = DiagnosisHighLatency
		message = "high round-trip latency detected"
	}

	report := &Report{
		Target:      target,
		Diagnosis:   diagnosis,
		LossRatio:   lossRatio,
		RTT:         rtt,
		FaultyHop:   faultyHop,
		FaultyHopIP: faultyHopIP,
		Message:     message,
		Timestamp:   time.Now(),
	}

	// Emit structured Zerolog log conforming to FR6
	logEvent := e.logger.Warn().
		Str("event", "network_diagnostic").
		Str("target", target).
		Str("diagnosis", string(diagnosis)).
		Float64("loss_ratio", lossRatio).
		Dur("rtt", rtt)

	if faultyHop > 0 {
		logEvent = logEvent.Int("faulty_hop", faultyHop).Str("faulty_hop_ip", faultyHopIP)
	}

	logEvent.Msg(message)

	return report
}
