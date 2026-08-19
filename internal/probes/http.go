package probes

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/devleesch001/stabsight/internal/config"
	"github.com/devleesch001/stabsight/internal/scheduler"
	"github.com/devleesch001/stabsight/internal/telemetry"
)

// HTTPProbeResult contains detailed timing breakdown of an HTTP request.
type HTTPProbeResult struct {
	TotalDuration      time.Duration
	TTFB               time.Duration
	TLSHandshake       time.Duration
	DNSDuration        time.Duration
	TCPConnectDuration time.Duration
	StatusCode         int
	Success            bool
}

// HTTPClient abstracts HTTP probe execution for testability.
type HTTPClient interface {
	Do(ctx context.Context, method, targetURL string, expectedCode int, tlsSkipVerify bool, timeout time.Duration) (*HTTPProbeResult, error)
}

// HTTPProbe implements scheduler.ProbeWorker for HTTP availability and latency checks.
type HTTPProbe struct {
	name          string
	targetName    string
	url           string
	method        string
	expectedCode  int
	tlsSkipVerify bool
	ipVersion     string
	interval      time.Duration
	timeout       time.Duration
	client        HTTPClient
	metrics       *telemetry.Metrics
	jitterCalc    *JitterCalculator
}

// NewHTTPProbe instantiates a new HTTPProbe worker.
func NewHTTPProbe(targetName, host string, cfg *config.HTTPProbeConfig, ipVersion string, metrics *telemetry.Metrics, client HTTPClient) *HTTPProbe {
	interval := 10 * time.Second
	timeout := 5 * time.Second
	urlStr := fmt.Sprintf("http://%s", host)
	method := "GET"
	expectedCode := 200
	tlsSkipVerify := false

	if cfg != nil {
		if cfg.Interval > 0 {
			interval = cfg.Interval
		}
		if cfg.Timeout > 0 {
			timeout = cfg.Timeout
		}
		if cfg.URL != "" {
			urlStr = cfg.URL
		}
		if cfg.Method != "" {
			method = strings.ToUpper(cfg.Method)
		}
		if cfg.ExpectedCode > 0 {
			expectedCode = cfg.ExpectedCode
		}
		tlsSkipVerify = cfg.TLSSkipVerify
	}

	if client == nil {
		client = &RealHTTPClient{}
	}

	return &HTTPProbe{
		name:          fmt.Sprintf("%s/http", targetName),
		targetName:    targetName,
		url:           urlStr,
		method:        method,
		expectedCode:  expectedCode,
		tlsSkipVerify: tlsSkipVerify,
		ipVersion:     ipVersion,
		interval:      interval,
		timeout:       timeout,
		client:        client,
		metrics:       metrics,
		jitterCalc:    NewJitterCalculator(),
	}
}

// Name returns the probe worker identifier.
func (p *HTTPProbe) Name() string { return p.name }

// TargetName returns the target name.
func (p *HTTPProbe) TargetName() string { return p.targetName }

// ProbeType returns "http".
func (p *HTTPProbe) ProbeType() string { return "http" }

// IsExclusive returns false.
func (p *HTTPProbe) IsExclusive() bool { return false }

// Start executes the periodic HTTP probe worker loop.
func (p *HTTPProbe) Start(ctx context.Context, cmdChan <-chan scheduler.Command, ackChan chan<- struct{}) error {
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
				select {
				case ackChan <- struct{}{}:
				case <-ctx.Done():
					return ctx.Err()
				}

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
			}

		case <-ticker.C:
			p.executeCheck(ctx)
		}
	}
}

// executeCheck runs the HTTP check and publishes OTel metrics.
func (p *HTTPProbe) executeCheck(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	res, err := p.client.Do(checkCtx, p.method, p.url, p.expectedCode, p.tlsSkipVerify, p.timeout)
	if err != nil || (res != nil && !res.Success) {
		extraAttrs := []attribute.KeyValue{}
		if res != nil && res.StatusCode > 0 {
			extraAttrs = append(extraAttrs, attribute.String("status_code", strconv.Itoa(res.StatusCode)))
		}
		if p.metrics != nil {
			p.metrics.RecordPacketLoss(ctx, 1.0, p.targetName, "http", p.ipVersion, extraAttrs...)
		}
		return
	}

	rttSeconds := res.TotalDuration.Seconds()
	extraAttrs := []attribute.KeyValue{
		attribute.String("status_code", strconv.Itoa(res.StatusCode)),
	}

	if jitter, ok := p.jitterCalc.Compute(rttSeconds); ok {
		if p.metrics != nil {
			p.metrics.RecordJitter(ctx, jitter, p.targetName, "http", p.ipVersion, extraAttrs...)
		}
	}

	if p.metrics != nil {
		p.metrics.RecordRTT(ctx, rttSeconds, p.targetName, "http", p.ipVersion, extraAttrs...)
		p.metrics.RecordPacketLoss(ctx, 0.0, p.targetName, "http", p.ipVersion, extraAttrs...)
	}
}

// RealHTTPClient performs HTTP requests instrumented with httptrace for TTFB and TLS measurements.
type RealHTTPClient struct{}

// Do executes an HTTP request and measures TTFB, TLS handshake, and total round-trip duration.
func (c *RealHTTPClient) Do(ctx context.Context, method, targetURL string, expectedCode int, tlsSkipVerify bool, timeout time.Duration) (*HTTPProbeResult, error) {
	req, err := http.NewRequestWithContext(ctx, method, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	var (
		dnsStart, dnsDone       time.Time
		connectStart, connectDone time.Time
		tlsStart, tlsDone         time.Time
		firstByteTime             time.Time
	)

	trace := &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:  func(_ httptrace.DNSDoneInfo) { dnsDone = time.Now() },
		ConnectStart: func(_, _ string) { connectStart = time.Now() },
		ConnectDone:  func(_, _ string, _ error) { connectDone = time.Now() },
		TLSHandshakeStart: func() { tlsStart = time.Now() },
		TLSHandshakeDone:  func(_ tls.ConnectionState, _ error) { tlsDone = time.Now() },
		GotFirstResponseByte: func() { firstByteTime = time.Now() },
	}

	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: tlsSkipVerify}, //nolint:gosec // user option for testing
		ResponseHeaderTimeout: timeout,
		DisableKeepAlives:     true, // Ensure clean full connection measurements
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	start := time.Now()
	resp, err := httpClient.Do(req)
	totalDuration := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	ttfb := time.Duration(0)
	if !firstByteTime.IsZero() {
		ttfb = firstByteTime.Sub(start)
	} else {
		ttfb = totalDuration
	}

	tlsDuration := time.Duration(0)
	if !tlsStart.IsZero() && !tlsDone.IsZero() {
		tlsDuration = tlsDone.Sub(tlsStart)
	}

	dnsDuration := time.Duration(0)
	if !dnsStart.IsZero() && !dnsDone.IsZero() {
		dnsDuration = dnsDone.Sub(dnsStart)
	}

	tcpDuration := time.Duration(0)
	if !connectStart.IsZero() && !connectDone.IsZero() {
		tcpDuration = connectDone.Sub(connectStart)
	}

	success := (expectedCode <= 0 && resp.StatusCode >= 200 && resp.StatusCode < 400) ||
		(expectedCode > 0 && resp.StatusCode == expectedCode)

	return &HTTPProbeResult{
		TotalDuration:      totalDuration,
		TTFB:               ttfb,
		TLSHandshake:       tlsDuration,
		DNSDuration:        dnsDuration,
		TCPConnectDuration: tcpDuration,
		StatusCode:         resp.StatusCode,
		Success:            success,
	}, nil
}
