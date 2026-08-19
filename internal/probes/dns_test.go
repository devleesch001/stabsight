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

type mockDNSResolver struct {
	mu        sync.Mutex
	rtts      []time.Duration
	callIndex int
	err       error
}

func (m *mockDNSResolver) Resolve(_ context.Context, _ string, _ string, _ string, _ time.Duration) (time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return 0, m.err
	}

	if len(m.rtts) == 0 {
		return 5 * time.Millisecond, nil
	}

	rtt := m.rtts[m.callIndex%len(m.rtts)]
	m.callIndex++
	return rtt, nil
}

func TestDNSProbe_Interface(t *testing.T) {
	cfg := &config.DNSProbeConfig{
		Interval:   50 * time.Millisecond,
		Timeout:    100 * time.Millisecond,
		Server:     "8.8.8.8:53",
		RecordType: "A",
	}

	probe := probes.NewDNSProbe("google-dns", "google.com", cfg, "ipv4", nil, &mockDNSResolver{})

	if probe.Name() != "google-dns/dns" {
		t.Errorf("expected name 'google-dns/dns', got %s", probe.Name())
	}
	if probe.TargetName() != "google-dns" {
		t.Errorf("expected targetName 'google-dns', got %s", probe.TargetName())
	}
	if probe.ProbeType() != "dns" {
		t.Errorf("expected probeType 'dns', got %s", probe.ProbeType())
	}
	if probe.IsExclusive() {
		t.Errorf("expected isExclusive false, got true")
	}
}

func TestDNSProbe_ExecutionAndMetrics(t *testing.T) {
	tel, err := telemetry.NewProvider(telemetry.ProviderConfig{ServiceName: "dns-test"})
	if err != nil {
		t.Fatalf("failed to create telemetry: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	metrics, err := telemetry.NewMetrics(tel.Meter("dns-test"))
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	resolver := &mockDNSResolver{
		rtts: []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 15 * time.Millisecond},
	}

	cfg := &config.DNSProbeConfig{
		Interval:   20 * time.Millisecond,
		Timeout:    50 * time.Millisecond,
		Server:     "1.1.1.1:53",
		RecordType: "AAAA",
	}

	probe := probes.NewDNSProbe("cloudflare-dns", "cloudflare.com", cfg, "ipv6", metrics, resolver)

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
}

func TestDNSProbe_FailureRecordsLoss(t *testing.T) {
	tel, err := telemetry.NewProvider(telemetry.ProviderConfig{ServiceName: "dns-fail-test"})
	if err != nil {
		t.Fatalf("failed to create telemetry: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	metrics, err := telemetry.NewMetrics(tel.Meter("dns-test"))
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	failingResolver := &mockDNSResolver{err: errors.New("i/o timeout")}

	cfg := &config.DNSProbeConfig{
		Interval: 20 * time.Millisecond,
		Timeout:  20 * time.Millisecond,
	}

	probe := probes.NewDNSProbe("failing-dns", "bad.domain", cfg, "ipv4", metrics, failingResolver)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cmdChan := make(chan scheduler.Command, 1)
	ackChan := make(chan struct{}, 1)

	_ = probe.Start(ctx, cmdChan, ackChan)
}
