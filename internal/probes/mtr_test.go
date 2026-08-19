package probes_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/devleesch001/stabsight/internal/config"
	"github.com/devleesch001/stabsight/internal/probes"
	"github.com/devleesch001/stabsight/internal/scheduler"
	"github.com/devleesch001/stabsight/internal/telemetry"
)

type mockMTRTracer struct {
	mu     sync.Mutex
	result *probes.MTRResult
	err    error
}

func (m *mockMTRTracer) Trace(_ context.Context, host string, _ int, _ time.Duration) (*probes.MTRResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return nil, m.err
	}

	if m.result != nil {
		return m.result, nil
	}

	return &probes.MTRResult{
		Target: host,
		Hops: []probes.MTRHop{
			{Hop: 1, IP: "192.168.1.1", RTT: 1 * time.Millisecond, Success: true},
			{Hop: 2, IP: "10.0.0.1", RTT: 5 * time.Millisecond, Success: true},
			{Hop: 3, IP: "8.8.8.8", RTT: 15 * time.Millisecond, Success: true},
		},
		ReachedTarget: true,
	}, nil
}

func TestMTRProbe_Interface(t *testing.T) {
	cfg := &config.MTRProbeConfig{
		Interval: 30 * time.Second,
		Timeout:  2 * time.Second,
		MaxHops:  15,
	}

	probe := probes.NewMTRProbe("google-mtr", "8.8.8.8", cfg, nil, &mockMTRTracer{}, nil)

	if probe.Name() != "google-mtr/mtr" {
		t.Errorf("expected name 'google-mtr/mtr', got %s", probe.Name())
	}
	if probe.TargetName() != "google-mtr" {
		t.Errorf("expected targetName 'google-mtr', got %s", probe.TargetName())
	}
	if probe.ProbeType() != "mtr" {
		t.Errorf("expected probeType 'mtr', got %s", probe.ProbeType())
	}
	if probe.IsExclusive() {
		t.Errorf("expected isExclusive false, got true")
	}
}

func TestMTRProbe_ExecutionAndCallback(t *testing.T) {
	tel, err := telemetry.NewProvider(telemetry.ProviderConfig{ServiceName: "mtr-test"})
	if err != nil {
		t.Fatalf("failed to create telemetry: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	metrics, err := telemetry.NewMetrics(tel.Meter("mtr-test"))
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	tracer := &mockMTRTracer{
		result: &probes.MTRResult{
			Target: "1.1.1.1",
			Hops: []probes.MTRHop{
				{Hop: 1, IP: "192.168.1.1", RTT: 2 * time.Millisecond, Success: true},
				{Hop: 2, IP: "*", Success: false},
				{Hop: 3, IP: "1.1.1.1", RTT: 12 * time.Millisecond, Success: true},
			},
			ReachedTarget: true,
		},
	}

	var receivedResult *probes.MTRResult
	var cbMu sync.Mutex

	cfg := &config.MTRProbeConfig{
		Interval: 20 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		MaxHops:  10,
	}

	probe := probes.NewMTRProbe("cloudflare", "1.1.1.1", cfg, metrics, tracer, func(res *probes.MTRResult) {
		cbMu.Lock()
		receivedResult = res
		cbMu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	cmdChan := make(chan scheduler.Command, 1)
	ackChan := make(chan struct{}, 1)

	done := make(chan error, 1)
	go func() {
		done <- probe.Start(ctx, cmdChan, ackChan)
	}()

	time.Sleep(50 * time.Millisecond)

	// Pause
	cmdChan <- scheduler.CmdPause
	select {
	case <-ackChan:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for pause ACK")
	}

	// Resume
	cmdChan <- scheduler.CmdResume
	time.Sleep(30 * time.Millisecond)

	// Stop
	cmdChan <- scheduler.CmdStop

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("unexpected probe error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("probe did not stop cleanly")
	}

	cbMu.Lock()
	res := receivedResult
	cbMu.Unlock()

	if res == nil {
		t.Fatal("expected callback to be called with MTRResult")
	}
	if len(res.Hops) != 3 {
		t.Errorf("expected 3 hops, got %d", len(res.Hops))
	}
	if probe.LastResult() == nil {
		t.Error("expected LastResult to be populated")
	}
}

func TestMTRProbe_ErrorHandling(_ *testing.T) {
	tracer := &mockMTRTracer{err: errors.New("traceroute failure")}

	cfg := &config.MTRProbeConfig{
		Interval: 20 * time.Millisecond,
		Timeout:  20 * time.Millisecond,
	}

	probe := probes.NewMTRProbe("err-mtr", "10.0.0.1", cfg, nil, tracer, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cmdChan := make(chan scheduler.Command, 1)
	ackChan := make(chan struct{}, 1)

	_ = probe.Start(ctx, cmdChan, ackChan)
}
