package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/devleesch001/stabsight/internal/scheduler"
)

// mockWorker is a test implementation of ProbeWorker.
type mockWorker struct {
	name        string
	targetName  string
	probeType   string
	isExclusive bool
	executed    int
	paused      bool
}

func (m *mockWorker) Name() string        { return m.name }
func (m *mockWorker) TargetName() string  { return m.targetName }
func (m *mockWorker) ProbeType() string   { return m.probeType }
func (m *mockWorker) IsExclusive() bool   { return m.isExclusive }

func (m *mockWorker) Start(ctx context.Context, cmdChan <-chan scheduler.Command, ackChan chan<- struct{}) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case cmd := <-cmdChan:
			switch cmd {
			case scheduler.CmdPause:
				m.paused = true
				// Acknowledge pause
				select {
				case ackChan <- struct{}{}:
				case <-ctx.Done():
					return ctx.Err()
				}
				// Wait for resume or context cancel
			pauseLoop:
				for {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case resumeCmd := <-cmdChan:
						if resumeCmd == scheduler.CmdResume {
							m.paused = false
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
				m.paused = false
			}
		case <-ticker.C:
			if !m.paused {
				m.executed++
			}
		}
	}
}

func TestProbeWorkerInterface(t *testing.T) {
	var worker scheduler.ProbeWorker = &mockWorker{
		name:        "test/icmp",
		targetName:  "test",
		probeType:   "icmp",
		isExclusive: false,
	}

	if worker.Name() != "test/icmp" {
		t.Errorf("expected name 'test/icmp', got %s", worker.Name())
	}
	if worker.TargetName() != "test" {
		t.Errorf("expected targetName 'test', got %s", worker.TargetName())
	}
	if worker.ProbeType() != "icmp" {
		t.Errorf("expected probeType 'icmp', got %s", worker.ProbeType())
	}
	if worker.IsExclusive() {
		t.Errorf("expected isExclusive false, got true")
	}
}
