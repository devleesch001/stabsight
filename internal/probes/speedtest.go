package probes

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/showwin/speedtest-go/speedtest"

	"github.com/devleesch001/stabsight/internal/config"
	"github.com/devleesch001/stabsight/internal/logging"
	"github.com/devleesch001/stabsight/internal/scheduler"
	"github.com/devleesch001/stabsight/internal/telemetry"
)

// SpeedtestRunner abstracts bandwidth measurements in bits per second.
type SpeedtestRunner interface {
	Run(ctx context.Context, serverID string) (downloadBitsPerSec float64, uploadBitsPerSec float64, err error)
}

// ExclusiveCoordinator abstracts scheduler exclusive execution to avoid bufferbloat (FR4).
type ExclusiveCoordinator interface {
	ExecuteExclusive(ctx context.Context, fn func(ctx context.Context) error) error
}

// SpeedtestProbe implements scheduler.ProbeWorker in exclusive mode.
type SpeedtestProbe struct {
	name        string
	targetName  string
	interval    time.Duration
	timeout     time.Duration
	serverID    string
	coordinator ExclusiveCoordinator
	runner      SpeedtestRunner
	metrics     *telemetry.Metrics
}

// NewSpeedtestProbe instantiates a new SpeedtestProbe worker.
func NewSpeedtestProbe(
	targetName string,
	cfg *config.SpeedtestProbeConfig,
	coordinator ExclusiveCoordinator,
	metrics *telemetry.Metrics,
	runner SpeedtestRunner,
) *SpeedtestProbe {
	interval := 30 * time.Minute
	timeout := 60 * time.Second
	serverID := ""

	if cfg != nil {
		if cfg.Interval > 0 {
			interval = cfg.Interval
		}
		if cfg.Timeout > 0 {
			timeout = cfg.Timeout
		}
		serverID = cfg.ServerID
	}

	if runner == nil {
		runner = &RealSpeedtestRunner{}
	}

	return &SpeedtestProbe{
		name:        fmt.Sprintf("%s/speedtest", targetName),
		targetName:  targetName,
		interval:    interval,
		timeout:     timeout,
		serverID:    serverID,
		coordinator: coordinator,
		runner:      runner,
		metrics:     metrics,
	}
}

// Name returns the probe worker identifier.
func (p *SpeedtestProbe) Name() string { return p.name }

// TargetName returns the target host label.
func (p *SpeedtestProbe) TargetName() string { return p.targetName }

// ProbeType returns "speedtest".
func (p *SpeedtestProbe) ProbeType() string { return "speedtest" }

// IsExclusive returns true (FR4: requires stopping all other probes during execution).
func (p *SpeedtestProbe) IsExclusive() bool { return true }

// Start executes the periodic speedtest worker loop.
func (p *SpeedtestProbe) Start(ctx context.Context, cmdChan <-chan scheduler.Command, _ chan<- struct{}) error {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// Run initial measurement
	p.triggerMeasurement(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case cmd := <-cmdChan:
			if cmd == scheduler.CmdStop {
				return nil
			}

		case <-ticker.C:
			p.triggerMeasurement(ctx)
		}
	}
}

// triggerMeasurement orchestrates exclusive execution if coordinator is available.
func (p *SpeedtestProbe) triggerMeasurement(ctx context.Context) {
	logger := logging.Get()
	logger.Info().Str("probe", p.name).Msg("Starting exclusive Speedtest bandwidth measurement (pausing background probes)")

	runFn := func(execCtx context.Context) error {
		return p.executeSpeedtest(execCtx)
	}

	var err error
	if p.coordinator != nil {
		err = p.coordinator.ExecuteExclusive(ctx, runFn)
	} else {
		err = runFn(ctx)
	}

	if err != nil {
		logger.Error().Err(err).Str("probe", p.name).Msg("Speedtest measurement failed")
	} else {
		logger.Info().Str("probe", p.name).Msg("Speedtest bandwidth measurement completed successfully")
	}
}

// executeSpeedtest performs the bandwidth test and publishes OTel metrics.
func (p *SpeedtestProbe) executeSpeedtest(ctx context.Context) error {
	testCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	dlBitsSec, ulBitsSec, err := p.runner.Run(testCtx, p.serverID)
	if err != nil {
		return err
	}

	if p.metrics != nil {
		p.metrics.RecordSpeedtest(ctx, dlBitsSec, p.targetName, "download")
		p.metrics.RecordSpeedtest(ctx, ulBitsSec, p.targetName, "upload")
	}

	logger := logging.Get()
	logger.Debug().
		Str("probe", p.name).
		Str("target", p.targetName).
		Float64("download_bits_per_sec", dlBitsSec).
		Float64("upload_bits_per_sec", ulBitsSec).
		Float64("download_mbps", dlBitsSec/1_000_000).
		Float64("upload_mbps", ulBitsSec/1_000_000).
		Msg("Speedtest bandwidth measurement details")

	return nil
}

// RealSpeedtestRunner performs native Speedtest testing using speedtest-go.
type RealSpeedtestRunner struct{}

// Run locates closest server and executes download and upload bandwidth tests in bits per second.
func (r *RealSpeedtestRunner) Run(ctx context.Context, serverID string) (float64, float64, error) {
	client := speedtest.New()

	// Fetch user info for location-based closest server selection
	_, _ = client.FetchUserInfo()

	serverList, err := client.FetchServers()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch speedtest servers: %w", err)
	}

	var targetServer *speedtest.Server
	if serverID != "" {
		if id, parseErr := strconv.Atoi(serverID); parseErr == nil {
			servers, _ := serverList.FindServer([]int{id})
			if len(servers) > 0 {
				targetServer = servers[0]
			}
		}
	}

	if targetServer == nil {
		targets, findErr := serverList.FindServer(nil)
		if findErr != nil || len(targets) == 0 {
			return 0, 0, fmt.Errorf("no available speedtest servers found: %w", findErr)
		}
		targetServer = targets[0]
	}

	if err := targetServer.PingTestContext(ctx, nil); err != nil {
		return 0, 0, fmt.Errorf("speedtest ping test failed: %w", err)
	}

	if err := targetServer.DownloadTestContext(ctx); err != nil {
		return 0, 0, fmt.Errorf("speedtest download test failed: %w", err)
	}

	if err := targetServer.UploadTestContext(ctx); err != nil {
		return 0, 0, fmt.Errorf("speedtest upload test failed: %w", err)
	}

	// Convert bytes/sec to bits/sec (1 Byte = 8 bits)
	dlBitsSec := float64(targetServer.DLSpeed) * 8
	ulBitsSec := float64(targetServer.ULSpeed) * 8

	return dlBitsSec, ulBitsSec, nil
}
