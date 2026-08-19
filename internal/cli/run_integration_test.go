package cli_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devleesch001/stabsight/internal/cli"
	"github.com/devleesch001/stabsight/internal/config"
	"github.com/devleesch001/stabsight/internal/logging"
)

func TestRunApp_FullLifecycle(t *testing.T) {
	_ = logging.Init("debug")

	// Local mock HTTP server
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer httpSrv.Close()

	// Local mock TCP listener
	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create TCP listener: %v", err)
	}
	defer func() { _ = tcpListener.Close() }()
	tcpPort := tcpListener.Addr().(*net.TCPAddr).Port

	go func() {
		for {
			conn, err := tcpListener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	// Find free port for Prometheus metrics
	freeL, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	metricsAddr := freeL.Addr().String()
	_ = freeL.Close()

	cfg := &config.Config{
		MetricsAddr: metricsAddr,
		LogLevel:    "debug",
		Targets: []config.Target{
			{
				Name:      "test-local-target",
				Host:      "127.0.0.1",
				IPVersion: "ipv4",
				Probes: config.ProbesConfig{
					HTTP: &config.HTTPProbeConfig{
						Interval:     50 * time.Millisecond,
						Timeout:      50 * time.Millisecond,
						URL:          httpSrv.URL,
						Method:       "GET",
						ExpectedCode: 200,
					},
					TCP: &config.TCPProbeConfig{
						Interval: 50 * time.Millisecond,
						Timeout:  50 * time.Millisecond,
						Port:     tcpPort,
					},
					DNS: &config.DNSProbeConfig{
						Interval: 50 * time.Millisecond,
						Timeout:  20 * time.Millisecond,
						Server:   "127.0.0.1:53",
					},
					ICMP: &config.ICMPProbeConfig{
						Interval: 50 * time.Millisecond,
						Timeout:  20 * time.Millisecond,
					},
					MTR: &config.MTRProbeConfig{
						Interval: 100 * time.Millisecond,
						Timeout:  20 * time.Millisecond,
						MaxHops:  1,
					},
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- cli.RunApp(ctx, cfg)
	}()

	// Wait for metrics server to start
	time.Sleep(50 * time.Millisecond)

	client := &http.Client{Timeout: 100 * time.Millisecond}
	resp, err := client.Get("http://" + metricsAddr + "/healthz")
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK on /healthz, got %d", resp.StatusCode)
		}
	}

	mResp, err := client.Get("http://" + metricsAddr + "/metrics")
	if err == nil {
		defer func() { _ = mResp.Body.Close() }()
		body, _ := io.ReadAll(mResp.Body)
		if !strings.Contains(string(body), "internet_monitor") {
			t.Logf("metrics body: %s", string(body))
		}
	}

	select {
	case appErr := <-errChan:
		if appErr != nil {
			t.Fatalf("RunApp returned unexpected error: %v", appErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunApp did not stop within timeout")
	}
}

func TestRunApp_InvalidMetricsAddr(t *testing.T) {
	cfg := &config.Config{
		MetricsAddr: "invalid:address:format:99999",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := cli.RunApp(ctx, cfg)
	if err == nil {
		t.Fatal("expected error with invalid metrics address, got nil")
	}
}
