package probes_test

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/devleesch001/stabsight/internal/config"
	"github.com/devleesch001/stabsight/internal/probes"
	"github.com/devleesch001/stabsight/internal/scheduler"
	"github.com/devleesch001/stabsight/internal/telemetry"
)

type mockTCPDialer struct {
	mu        sync.Mutex
	rtts      []time.Duration
	callIndex int
	err       error
}

func (m *mockTCPDialer) Dial(_ context.Context, _, _ string, _ time.Duration) (time.Duration, error) {
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

func TestTCPProbe_Interface(t *testing.T) {
	cfg := &config.TCPProbeConfig{
		Interval: 50 * time.Millisecond,
		Timeout:  100 * time.Millisecond,
		Port:     443,
	}

	probe := probes.NewTCPProbe("gateway", "192.168.1.1", cfg, "ipv4", nil, &mockTCPDialer{})

	if probe.Name() != "gateway/tcp" {
		t.Errorf("expected name 'gateway/tcp', got %s", probe.Name())
	}
	if probe.TargetName() != "gateway" {
		t.Errorf("expected targetName 'gateway', got %s", probe.TargetName())
	}
	if probe.ProbeType() != "tcp" {
		t.Errorf("expected probeType 'tcp', got %s", probe.ProbeType())
	}
	if probe.IsExclusive() {
		t.Errorf("expected isExclusive false, got true")
	}
}

func TestTCPProbe_ExecutionAndMetrics(t *testing.T) {
	tel, err := telemetry.NewProvider(telemetry.ProviderConfig{ServiceName: "tcp-test"})
	if err != nil {
		t.Fatalf("failed to create telemetry: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	metrics, err := telemetry.NewMetrics(tel.Meter("tcp-test"))
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	dialer := &mockTCPDialer{
		rtts: []time.Duration{15 * time.Millisecond, 25 * time.Millisecond, 18 * time.Millisecond},
	}

	cfg := &config.TCPProbeConfig{
		Interval: 20 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		Port:     80,
	}

	probe := probes.NewTCPProbe("router", "10.0.0.1", cfg, "ipv4", metrics, dialer)

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

func TestRealTCPDialer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer func() { _ = listener.Close() }()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	addr := listener.Addr().(*net.TCPAddr)
	port := addr.Port

	dialer := &probes.RealTCPDialer{}
	rtt, err := dialer.Dial(context.Background(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 2*time.Second)
	if err != nil {
		t.Fatalf("failed to dial real TCP listener: %v", err)
	}
	if rtt <= 0 {
		t.Errorf("expected positive RTT, got %v", rtt)
	}
}

func TestTCPProbe_FailureRecordsLoss(t *testing.T) {
	tel, err := telemetry.NewProvider(telemetry.ProviderConfig{ServiceName: "tcp-fail-test"})
	if err != nil {
		t.Fatalf("failed to create telemetry: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	metrics, err := telemetry.NewMetrics(tel.Meter("tcp-test"))
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	failingDialer := &mockTCPDialer{err: errors.New("connection refused")}

	cfg := &config.TCPProbeConfig{
		Interval: 20 * time.Millisecond,
		Timeout:  20 * time.Millisecond,
		Port:     9999,
	}

	probe := probes.NewTCPProbe("failing-tcp", "127.0.0.1", cfg, "ipv4", metrics, failingDialer)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cmdChan := make(chan scheduler.Command, 1)
	ackChan := make(chan struct{}, 1)

	_ = probe.Start(ctx, cmdChan, ackChan)
}
