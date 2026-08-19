package scheduler_test

import (
	"context"
	"testing"

	"github.com/devleesch001/stabsight/internal/scheduler"
)

type dummyWorker struct {
	name        string
	targetName  string
	probeType   string
	isExclusive bool
}

func (d *dummyWorker) Name() string       { return d.name }
func (d *dummyWorker) TargetName() string { return d.targetName }
func (d *dummyWorker) ProbeType() string  { return d.probeType }
func (d *dummyWorker) IsExclusive() bool  { return d.isExclusive }
func (d *dummyWorker) Start(_ context.Context, _ <-chan scheduler.Command, _ chan<- struct{}) error {
	return nil
}

func TestSchedulerRegisterWorkers(t *testing.T) {
	sched := scheduler.NewScheduler()

	if sched.WorkerCount() != 0 {
		t.Fatalf("expected 0 workers, got %d", sched.WorkerCount())
	}
	if sched.IsRunning() {
		t.Fatal("expected scheduler not to be running initially")
	}

	w1 := &dummyWorker{name: "w1", targetName: "t1", probeType: "icmp"}
	w2 := &dummyWorker{name: "w2", targetName: "t2", probeType: "dns"}

	if err := sched.RegisterWorker(w1); err != nil {
		t.Fatalf("failed to register w1: %v", err)
	}
	if err := sched.RegisterWorker(w2); err != nil {
		t.Fatalf("failed to register w2: %v", err)
	}

	if sched.WorkerCount() != 2 {
		t.Fatalf("expected 2 workers, got %d", sched.WorkerCount())
	}

	workers := sched.Workers()
	if len(workers) != 2 {
		t.Fatalf("expected 2 workers in slice, got %d", len(workers))
	}
	if workers[0].Name() != "w1" || workers[1].Name() != "w2" {
		t.Errorf("unexpected workers order: %v, %v", workers[0].Name(), workers[1].Name())
	}
}

func TestSchedulerRegisterErrors(t *testing.T) {
	sched := scheduler.NewScheduler()

	// Nil worker
	if err := sched.RegisterWorker(nil); err == nil {
		t.Fatal("expected error registering nil worker, got nil")
	}

	w1 := &dummyWorker{name: "w1", targetName: "t1", probeType: "icmp"}
	if err := sched.RegisterWorker(w1); err != nil {
		t.Fatalf("failed to register w1: %v", err)
	}

	// Duplicate name
	duplicate := &dummyWorker{name: "w1", targetName: "t2", probeType: "dns"}
	if err := sched.RegisterWorker(duplicate); err == nil {
		t.Fatal("expected error registering duplicate worker name, got nil")
	}
}

func TestSchedulerRegisterBatch(t *testing.T) {
	sched := scheduler.NewScheduler()

	w1 := &dummyWorker{name: "batch-1", targetName: "t1", probeType: "icmp"}
	w2 := &dummyWorker{name: "batch-2", targetName: "t2", probeType: "dns"}

	if err := sched.RegisterWorkers(w1, w2); err != nil {
		t.Fatalf("failed to register batch workers: %v", err)
	}

	if sched.WorkerCount() != 2 {
		t.Fatalf("expected 2 workers, got %d", sched.WorkerCount())
	}
}
