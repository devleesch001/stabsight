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

type mockPinger struct {
	mu        sync.Mutex
	rtts      []time.Duration
	callIndex int
	err       error
}

func (m *mockPinger) Ping(_ context.Context, _ string, _ string, _ time.Duration) (time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return 0, m.err
	}

	if len(m.rtts) == 0 {
		return 10 * time.Millisecond, nil
	}

	rtt := m.rtts[m.callIndex%len(m.rtts)]
	m.callIndex++
	return rtt, nil
}

func TestICMPProbe_Interface(t *testing.T) {
	cfg := &config.ICMPProbeConfig{
		Interval: 50 * time.Millisecond,
		Timeout:  100 * time.Millisecond,
		Count:    1,
	}

	p := probes.NewICMPProbe("google-dns", "8.8.8.8", cfg, "ipv4", nil, &mockPinger{})

	if p.Name() != "google-dns/icmp" {
		t.Errorf("expected name 'google-dns/icmp', got %s", p.Name())
	}
	if p.TargetName() != "google-dns" {
		t.Errorf("expected targetName 'google-dns', got %s", p.TargetName())
	}
	if p.ProbeType() != "icmp" {
		t.Errorf("expected probeType 'icmp', got %s", p.ProbeType())
	}
	if p.IsExclusive() {
		t.Errorf("expected isExclusive false, got true")
	}
}

func TestICMPProbe_ExecutionAndMetrics(t *testing.T) {
	tel, err := telemetry.NewProvider(telemetry.ProviderConfig{ServiceName: "icmp-test"})
	if err != nil {
		t.Fatalf("failed to create telemetry: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	metrics, err := telemetry.NewMetrics(tel.Meter("icmp-test"))
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	pinger := &mockPinger{
		rtts: []time.Duration{10 * time.Millisecond, 15 * time.Millisecond, 12 * time.Millisecond},
	}

	cfg := &config.ICMPProbeConfig{
		Interval: 20 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		Count:    1,
	}

	probe := probes.NewICMPProbe("google-dns", "8.8.8.8", cfg, "ipv4", metrics, pinger)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	cmdChan := make(chan scheduler.Command, 1)
	ackChan := make(chan struct{}, 1)

	// Run probe worker in goroutine
	done := make(chan error, 1)
	go func() {
		done <- probe.Start(ctx, cmdChan, ackChan)
	}()

	// Wait for a few ticks
	time.Sleep(50 * time.Millisecond)

	// Test pause command
	cmdChan <- scheduler.CmdPause
	select {
	case <-ackChan:
		// Ack received
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for pause ACK")
	}

	// Test resume command
	cmdChan <- scheduler.CmdResume

	// Let it run a bit more
	time.Sleep(30 * time.Millisecond)

	// Stop command
	cmdChan <- scheduler.CmdStop

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("unexpected probe error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("probe did not terminate after stop")
	}
}

func TestICMPProbe_FailureRecordsLoss(t *testing.T) {
	tel, err := telemetry.NewProvider(telemetry.ProviderConfig{ServiceName: "icmp-fail-test"})
	if err != nil {
		t.Fatalf("failed to create telemetry: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	metrics, err := telemetry.NewMetrics(tel.Meter("icmp-test"))
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	failingPinger := &mockPinger{err: errors.New("network unreachable")}

	cfg := &config.ICMPProbeConfig{
		Interval: 20 * time.Millisecond,
		Timeout:  20 * time.Millisecond,
	}

	probe := probes.NewICMPProbe("unreachable", "192.0.2.1", cfg, "ipv4", metrics, failingPinger)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cmdChan := make(chan scheduler.Command, 1)
	ackChan := make(chan struct{}, 1)

	_ = probe.Start(ctx, cmdChan, ackChan)
}
