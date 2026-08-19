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

type mockSpeedtestRunner struct {
	mu           sync.Mutex
	downloadRate float64
	uploadRate   float64
	err          error
	callCount    int
}

func (m *mockSpeedtestRunner) Run(_ context.Context, _ string) (float64, float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	if m.err != nil {
		return 0, 0, m.err
	}
	return m.downloadRate, m.uploadRate, nil
}

type mockCoordinator struct {
	mu        sync.Mutex
	callCount int
}

func (m *mockCoordinator) ExecuteExclusive(ctx context.Context, fn func(ctx context.Context) error) error {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()
	return fn(ctx)
}

func TestSpeedtestProbe_Interface(t *testing.T) {
	cfg := &config.SpeedtestProbeConfig{
		Interval: 10 * time.Minute,
		Timeout:  30 * time.Second,
		ServerID: "1234",
	}

	probe := probes.NewSpeedtestProbe("speed-test-target", cfg, nil, nil, &mockSpeedtestRunner{})

	if probe.Name() != "speed-test-target/speedtest" {
		t.Errorf("expected name 'speed-test-target/speedtest', got %s", probe.Name())
	}
	if probe.TargetName() != "speed-test-target" {
		t.Errorf("expected targetName 'speed-test-target', got %s", probe.TargetName())
	}
	if probe.ProbeType() != "speedtest" {
		t.Errorf("expected probeType 'speedtest', got %s", probe.ProbeType())
	}
	if !probe.IsExclusive() {
		t.Errorf("expected isExclusive true, got false")
	}
}

func TestSpeedtestProbe_ExecutionWithCoordinatorAndMetrics(t *testing.T) {
	tel, err := telemetry.NewProvider(telemetry.ProviderConfig{ServiceName: "speedtest-test"})
	if err != nil {
		t.Fatalf("failed to create telemetry: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	metrics, err := telemetry.NewMetrics(tel.Meter("speedtest-test"))
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	runner := &mockSpeedtestRunner{
		downloadRate: 125000000.0, // 1 Gbps = 125 MB/s
		uploadRate:   50000000.0,  // 400 Mbps = 50 MB/s
	}

	coord := &mockCoordinator{}

	cfg := &config.SpeedtestProbeConfig{
		Interval: 20 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
	}

	probe := probes.NewSpeedtestProbe("local-speed", cfg, coord, metrics, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	cmdChan := make(chan scheduler.Command, 1)
	ackChan := make(chan struct{}, 1)

	done := make(chan error, 1)
	go func() {
		done <- probe.Start(ctx, cmdChan, ackChan)
	}()

	time.Sleep(50 * time.Millisecond)
	cmdChan <- scheduler.CmdStop

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("unexpected probe error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("probe did not stop cleanly")
	}

	coord.mu.Lock()
	calls := coord.callCount
	coord.mu.Unlock()

	if calls == 0 {
		t.Error("expected coordinator.ExecuteExclusive to be invoked at least once")
	}
}

func TestSpeedtestProbe_ErrorHandling(_ *testing.T) {
	runner := &mockSpeedtestRunner{
		err: errors.New("speedtest server timeout"),
	}

	cfg := &config.SpeedtestProbeConfig{
		Interval: 20 * time.Millisecond,
		Timeout:  20 * time.Millisecond,
	}

	probe := probes.NewSpeedtestProbe("error-speed", cfg, nil, nil, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cmdChan := make(chan scheduler.Command, 1)
	ackChan := make(chan struct{}, 1)

	_ = probe.Start(ctx, cmdChan, ackChan)
}
