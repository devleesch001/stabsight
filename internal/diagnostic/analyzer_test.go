package diagnostic_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/devleesch001/stabsight/internal/diagnostic"
	"github.com/devleesch001/stabsight/internal/probes"
)

func TestEngine_AnalyzeNormal(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	engine := diagnostic.NewEngine(logger, 0.05, 100*time.Millisecond)

	mtr := &probes.MTRResult{
		Target: "8.8.8.8",
		Hops: []probes.MTRHop{
			{Hop: 1, IP: "192.168.1.1", RTT: 1 * time.Millisecond, Success: true},
			{Hop: 2, IP: "10.0.0.1", RTT: 10 * time.Millisecond, Success: true},
		},
		ReachedTarget: true,
	}

	report := engine.Analyze("8.8.8.8", 0.0, 15*time.Millisecond, mtr)
	if report.Diagnosis != diagnostic.DiagnosisOK {
		t.Errorf("expected DiagnosisOK, got %s", report.Diagnosis)
	}

	if buf.Len() > 0 {
		t.Errorf("expected no log for normal status, got %s", buf.String())
	}
}

func TestEngine_AnalyzeLocalNetworkFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	engine := diagnostic.NewEngine(logger, 0.05, 100*time.Millisecond)

	mtr := &probes.MTRResult{
		Target: "8.8.8.8",
		Hops: []probes.MTRHop{
			{Hop: 1, IP: "192.168.1.1", Success: false},
		},
		ReachedTarget: false,
	}

	report := engine.Analyze("8.8.8.8", 0.20, 0, mtr)
	if report.Diagnosis != diagnostic.DiagnosisLocalNetwork {
		t.Errorf("expected DiagnosisLocalNetwork, got %s", report.Diagnosis)
	}
	if report.FaultyHop != 1 {
		t.Errorf("expected faulty hop 1, got %d", report.FaultyHop)
	}

	logStr := buf.String()
	if !strings.Contains(logStr, "local_network_issue") || !strings.Contains(logStr, "network_diagnostic") {
		t.Errorf("expected structured log with local_network_issue, got: %s", logStr)
	}
}

func TestEngine_AnalyzeISPGatewayFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	engine := diagnostic.NewEngine(logger, 0.05, 100*time.Millisecond)

	mtr := &probes.MTRResult{
		Target: "1.1.1.1",
		Hops: []probes.MTRHop{
			{Hop: 1, IP: "192.168.1.1", RTT: 1 * time.Millisecond, Success: true},
			{Hop: 2, IP: "80.10.20.1", Success: false},
		},
		ReachedTarget: false,
	}

	report := engine.Analyze("1.1.1.1", 0.15, 0, mtr)
	if report.Diagnosis != diagnostic.DiagnosisISPGateway {
		t.Errorf("expected DiagnosisISPGateway, got %s", report.Diagnosis)
	}
	if report.FaultyHop != 2 {
		t.Errorf("expected faulty hop 2, got %d", report.FaultyHop)
	}
}

func TestEngine_AnalyzeTransitLoss(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	engine := diagnostic.NewEngine(logger, 0.05, 100*time.Millisecond)

	mtr := &probes.MTRResult{
		Target: "example.com",
		Hops: []probes.MTRHop{
			{Hop: 1, IP: "192.168.1.1", RTT: 1 * time.Millisecond, Success: true},
			{Hop: 2, IP: "80.10.20.1", RTT: 5 * time.Millisecond, Success: true},
			{Hop: 3, IP: "80.10.20.254", RTT: 8 * time.Millisecond, Success: true},
			{Hop: 4, IP: "193.251.128.1", RTT: 15 * time.Millisecond, Success: true},
			{Hop: 5, IP: "193.251.128.2", Success: false},
		},
		ReachedTarget: false,
	}

	report := engine.Analyze("example.com", 0.10, 0, mtr)
	if report.Diagnosis != diagnostic.DiagnosisTransitLoss {
		t.Errorf("expected DiagnosisTransitLoss, got %s", report.Diagnosis)
	}
	if report.FaultyHop != 5 {
		t.Errorf("expected faulty hop 5, got %d", report.FaultyHop)
	}
}

func TestEngine_AnalyzeTargetUnreachableWithoutMTR(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	engine := diagnostic.NewEngine(logger, 0.05, 100*time.Millisecond)

	report := engine.Analyze("down-target", 1.0, 0, nil)
	if report.Diagnosis != diagnostic.DiagnosisTargetUnreachable {
		t.Errorf("expected DiagnosisTargetUnreachable, got %s", report.Diagnosis)
	}
}
