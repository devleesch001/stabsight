package scheduler_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devleesch001/stabsight/internal/scheduler"
)

// activeMockWorker simulates a probe worker with execution counting and pause/ack behavior.
type activeMockWorker struct {
	name        string
	targetName  string
	probeType   string
	isExclusive bool
	count       int64
	pauseCount  int64
	resumeCount int64
}

func newActiveMockWorker(name string, exclusive bool) *activeMockWorker {
	return &activeMockWorker{
		name:        name,
		targetName:  "target-" + name,
		probeType:   "mock",
		isExclusive: exclusive,
	}
}

func (m *activeMockWorker) Name() string       { return m.name }
func (m *activeMockWorker) TargetName() string { return m.targetName }
func (m *activeMockWorker) ProbeType() string  { return m.probeType }
func (m *activeMockWorker) IsExclusive() bool  { return m.isExclusive }

func (m *activeMockWorker) Start(ctx context.Context, cmdChan <-chan scheduler.Command, ackChan chan<- struct{}) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case cmd := <-cmdChan:
			switch cmd {
			case scheduler.CmdPause:
				atomic.AddInt64(&m.pauseCount, 1)
				// Finish in-flight request and acknowledge
				select {
				case ackChan <- struct{}{}:
				case <-ctx.Done():
					return ctx.Err()
				}
				// Wait in pause loop until Resume or Context Cancel
			pauseLoop:
				for {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case resumeCmd := <-cmdChan:
						if resumeCmd == scheduler.CmdResume {
							atomic.AddInt64(&m.resumeCount, 1)
							break pauseLoop
						}
					}
				}
			case scheduler.CmdStop:
				return nil
			}
		case <-ticker.C:
			atomic.AddInt64(&m.count, 1)
		}
	}
}

func TestSchedulerStartAndStop(t *testing.T) {
	sched := scheduler.NewScheduler()
	w1 := newActiveMockWorker("w1", false)
	w2 := newActiveMockWorker("w2", false)

	if err := sched.RegisterWorkers(w1, w2); err != nil {
		t.Fatalf("failed to register workers: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sched.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}

	if !sched.IsRunning() {
		t.Fatal("expected scheduler to be running")
	}

	// Cannot start twice
	if err := sched.Start(ctx); err == nil {
		t.Fatal("expected error starting already running scheduler, got nil")
	}

	// Wait for workers to tick
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt64(&w1.count) == 0 || atomic.LoadInt64(&w2.count) == 0 {
		t.Errorf("expected workers to have executed ticks, got w1=%d, w2=%d", w1.count, w2.count)
	}

	sched.Stop()
	if sched.IsRunning() {
		t.Fatal("expected scheduler to be stopped")
	}
}

func TestSchedulerPauseAndResume(t *testing.T) {
	sched := scheduler.NewScheduler()
	w1 := newActiveMockWorker("w1", false)
	w2 := newActiveMockWorker("w2", false)
	wExclusive := newActiveMockWorker("w-exclusive", true)

	if err := sched.RegisterWorkers(w1, w2, wExclusive); err != nil {
		t.Fatalf("failed to register workers: %v", err)
	}

	ctx := context.Background()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
	defer sched.Stop()

	// Let workers run briefly
	time.Sleep(40 * time.Millisecond)

	// Pause all non-exclusive workers
	pauseCtx, pauseCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer pauseCancel()

	if err := sched.PauseAll(pauseCtx); err != nil {
		t.Fatalf("failed to pause all workers: %v", err)
	}

	// Capture count at pause
	count1AtPause := atomic.LoadInt64(&w1.count)
	count2AtPause := atomic.LoadInt64(&w2.count)

	if atomic.LoadInt64(&w1.pauseCount) != 1 || atomic.LoadInt64(&w2.pauseCount) != 1 {
		t.Errorf("expected pauseCount 1, got w1=%d, w2=%d", w1.pauseCount, w2.pauseCount)
	}

	// Sleep while paused to verify no ticks increment
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt64(&w1.count) != count1AtPause || atomic.LoadInt64(&w2.count) != count2AtPause {
		t.Errorf("worker counts changed during pause: w1 (%d -> %d), w2 (%d -> %d)",
			count1AtPause, w1.count, count2AtPause, w2.count)
	}

	// Resume workers
	resumeCtx, resumeCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer resumeCancel()

	if err := sched.ResumeAll(resumeCtx); err != nil {
		t.Fatalf("failed to resume all workers: %v", err)
	}

	// Let workers resume ticking
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt64(&w1.count) <= count1AtPause || atomic.LoadInt64(&w2.count) <= count2AtPause {
		t.Errorf("workers did not resume counting: w1=%d (was %d), w2=%d (was %d)",
			w1.count, count1AtPause, w2.count, count2AtPause)
	}
	if atomic.LoadInt64(&w1.resumeCount) != 1 || atomic.LoadInt64(&w2.resumeCount) != 1 {
		t.Errorf("expected resumeCount 1, got w1=%d, w2=%d", w1.resumeCount, w2.resumeCount)
	}
}

func TestSchedulerExecuteExclusive(t *testing.T) {
	sched := scheduler.NewScheduler()
	w1 := newActiveMockWorker("w1", false)
	w2 := newActiveMockWorker("w2", false)

	if err := sched.RegisterWorkers(w1, w2); err != nil {
		t.Fatalf("failed to register workers: %v", err)
	}

	if err := sched.Start(context.Background()); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
	defer sched.Stop()

	time.Sleep(30 * time.Millisecond)

	var countDuringExclusive int64
	err := sched.ExecuteExclusive(context.Background(), func(_ context.Context) error {
		// Verify workers were paused
		c1 := atomic.LoadInt64(&w1.count)
		time.Sleep(40 * time.Millisecond)
		c2 := atomic.LoadInt64(&w1.count)

		if c1 != c2 {
			t.Errorf("worker w1 ticked during exclusive execution: %d -> %d", c1, c2)
		}
		countDuringExclusive = c1
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error during ExecuteExclusive: %v", err)
	}

	// Verify workers resumed after exclusive execution
	time.Sleep(40 * time.Millisecond)
	if atomic.LoadInt64(&w1.count) <= countDuringExclusive {
		t.Errorf("worker did not resume after ExecuteExclusive: current %d <= recorded %d",
			w1.count, countDuringExclusive)
	}
}

// uncooperativeWorker ignores pause command to test timeout handling.
type uncooperativeWorker struct {
	name string
}

func (u *uncooperativeWorker) Name() string       { return u.name }
func (u *uncooperativeWorker) TargetName() string { return "test" }
func (u *uncooperativeWorker) ProbeType() string  { return "mock" }
func (u *uncooperativeWorker) IsExclusive() bool  { return false }
func (u *uncooperativeWorker) Start(ctx context.Context, _ <-chan scheduler.Command, _ chan<- struct{}) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestSchedulerPauseTimeout(t *testing.T) {
	sched := scheduler.NewScheduler()
	badWorker := &uncooperativeWorker{name: "bad-worker"}

	if err := sched.RegisterWorker(badWorker); err != nil {
		t.Fatalf("failed to register bad worker: %v", err)
	}

	if err := sched.Start(context.Background()); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
	defer sched.Stop()

	// Short timeout
	pauseCtx, pauseCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer pauseCancel()

	err := sched.PauseAll(pauseCtx)
	if err == nil {
		t.Fatal("expected error pausing uncooperative worker, got nil")
	}
}
