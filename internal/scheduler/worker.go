package scheduler

import "context"

// ProbeWorker defines the lifecycle and control interface for network monitoring probes.
type ProbeWorker interface {
	// Name returns the unique worker identifier (e.g., "google-dns/icmp").
	Name() string

	// TargetName returns the target host or label associated with this probe.
	TargetName() string

	// ProbeType returns the probe protocol/type (e.g., "icmp", "dns", "http", "tcp", "mtr", "speedtest").
	ProbeType() string

	// IsExclusive returns true if the probe requires exclusive execution (e.g., Speedtest),
	// causing all other active workers to pause during its run.
	IsExclusive() bool

	// Start starts the worker execution loop.
	// It must listen to cmdChan for control commands (CmdPause, CmdResume, CmdStop)
	// and notify ackChan when a pause state is safely entered.
	Start(ctx context.Context, cmdChan <-chan Command, ackChan chan<- struct{}) error
}
