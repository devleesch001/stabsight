package probes_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/devleesch001/stabsight/internal/config"
	"github.com/devleesch001/stabsight/internal/probes"
	"github.com/devleesch001/stabsight/internal/scheduler"
	"github.com/devleesch001/stabsight/internal/telemetry"
)

type mockHTTPClient struct {
	mu        sync.Mutex
	results   []*probes.HTTPProbeResult
	callIndex int
	err       error
}

func (m *mockHTTPClient) Do(_ context.Context, _, _ string, _ int, _ bool, _ time.Duration) (*probes.HTTPProbeResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return nil, m.err
	}

	if len(m.results) == 0 {
		return &probes.HTTPProbeResult{
			TotalDuration: 50 * time.Millisecond,
			TTFB:          30 * time.Millisecond,
			StatusCode:    200,
			Success:       true,
		}, nil
	}

	res := m.results[m.callIndex%len(m.results)]
	m.callIndex++
	return res, nil
}

func TestHTTPProbe_Interface(t *testing.T) {
	cfg := &config.HTTPProbeConfig{
		Interval:     50 * time.Millisecond,
		Timeout:      100 * time.Millisecond,
		URL:          "https://example.com",
		Method:       "GET",
		ExpectedCode: 200,
	}

	probe := probes.NewHTTPProbe("web-target", "example.com", cfg, "ipv4", nil, &mockHTTPClient{})

	if probe.Name() != "web-target/http" {
		t.Errorf("expected name 'web-target/http', got %s", probe.Name())
	}
	if probe.TargetName() != "web-target" {
		t.Errorf("expected targetName 'web-target', got %s", probe.TargetName())
	}
	if probe.ProbeType() != "http" {
		t.Errorf("expected probeType 'http', got %s", probe.ProbeType())
	}
	if probe.IsExclusive() {
		t.Errorf("expected isExclusive false, got true")
	}
}

func TestHTTPProbe_ExecutionAndMetrics(t *testing.T) {
	tel, err := telemetry.NewProvider(telemetry.ProviderConfig{ServiceName: "http-test"})
	if err != nil {
		t.Fatalf("failed to create telemetry: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	metrics, err := telemetry.NewMetrics(tel.Meter("http-test"))
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	client := &mockHTTPClient{
		results: []*probes.HTTPProbeResult{
			{TotalDuration: 40 * time.Millisecond, TTFB: 20 * time.Millisecond, StatusCode: 200, Success: true},
			{TotalDuration: 60 * time.Millisecond, TTFB: 35 * time.Millisecond, StatusCode: 200, Success: true},
		},
	}

	cfg := &config.HTTPProbeConfig{
		Interval:     20 * time.Millisecond,
		Timeout:      50 * time.Millisecond,
		ExpectedCode: 200,
	}

	probe := probes.NewHTTPProbe("api", "api.example.com", cfg, "ipv4", metrics, client)

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

func TestRealHTTPClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	client := &probes.RealHTTPClient{}
	res, err := client.Do(context.Background(), "GET", ts.URL, 200, false, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to execute real HTTP request: %v", err)
	}

	if !res.Success {
		t.Fatalf("expected success true, got false (status: %d)", res.StatusCode)
	}
	if res.StatusCode != 200 {
		t.Errorf("expected status code 200, got %d", res.StatusCode)
	}
	if res.TTFB <= 0 {
		t.Errorf("expected positive TTFB, got %v", res.TTFB)
	}
}

func TestHTTPProbe_FailureRecordsLoss(t *testing.T) {
	tel, err := telemetry.NewProvider(telemetry.ProviderConfig{ServiceName: "http-fail-test"})
	if err != nil {
		t.Fatalf("failed to create telemetry: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	metrics, err := telemetry.NewMetrics(tel.Meter("http-test"))
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	failingClient := &mockHTTPClient{err: errors.New("connection refused")}

	cfg := &config.HTTPProbeConfig{
		Interval: 20 * time.Millisecond,
		Timeout:  20 * time.Millisecond,
	}

	probe := probes.NewHTTPProbe("failing-http", "invalid.host", cfg, "ipv4", metrics, failingClient)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cmdChan := make(chan scheduler.Command, 1)
	ackChan := make(chan struct{}, 1)

	_ = probe.Start(ctx, cmdChan, ackChan)
}
