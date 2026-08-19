package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type workerHandle struct {
	worker  ProbeWorker
	cmdChan chan Command
	ackChan chan struct{}
}

// Scheduler coordinates the lifecycle, concurrency, and exclusive execution of network probe workers.
type Scheduler struct {
	mu          sync.RWMutex
	workers     []*workerHandle
	running     bool
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	exclusiveMu sync.Mutex
	paused      bool
}

// NewScheduler instantiates a new Scheduler.
func NewScheduler() *Scheduler {
	return &Scheduler{
		workers: make([]*workerHandle, 0),
	}
}

// RegisterWorker adds a probe worker to the scheduler.
// It fails if the scheduler is already running, if worker is nil, or if the name is a duplicate.
func (s *Scheduler) RegisterWorker(w ProbeWorker) error {
	if w == nil {
		return errors.New("cannot register nil worker")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return errors.New("cannot register worker while scheduler is running")
	}

	for _, h := range s.workers {
		if h.worker.Name() == w.Name() {
			return fmt.Errorf("worker with name %q is already registered", w.Name())
		}
	}

	s.workers = append(s.workers, &workerHandle{
		worker:  w,
		cmdChan: make(chan Command, 1),
		ackChan: make(chan struct{}, 1),
	})

	return nil
}

// RegisterWorkers registers multiple probe workers sequentially.
func (s *Scheduler) RegisterWorkers(workers ...ProbeWorker) error {
	for _, w := range workers {
		if err := s.RegisterWorker(w); err != nil {
			return err
		}
	}
	return nil
}

// Workers returns a slice containing all currently registered probe workers.
func (s *Scheduler) Workers() []ProbeWorker {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ProbeWorker, len(s.workers))
	for i, h := range s.workers {
		result[i] = h.worker
	}
	return result
}

// WorkerCount returns the total number of registered probe workers.
func (s *Scheduler) WorkerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.workers)
}

// IsRunning reports whether the scheduler loop is currently active.
func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Start launches all non-exclusive probe workers concurrently in their own goroutines.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("scheduler is already running")
	}

	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.running = true
	s.paused = false
	s.mu.Unlock()

	s.mu.RLock()
	for _, h := range s.workers {
		// Exclusive workers (like Speedtest) are triggered separately via ExecuteExclusive
		if !h.worker.IsExclusive() {
			s.wg.Add(1)
			go func(handle *workerHandle) {
				defer s.wg.Done()
				_ = handle.worker.Start(runCtx, handle.cmdChan, handle.ackChan)
			}(h)
		}
	}
	s.mu.RUnlock()

	return nil
}

// PauseAll broadcasts CmdPause to all non-exclusive workers and waits for their ACKs.
// It returns an error if ctx is canceled before all ACKs arrive.
func (s *Scheduler) PauseAll(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return errors.New("scheduler is not running")
	}
	if s.paused {
		s.mu.Unlock()
		return nil
	}

	nonExclusiveHandles := make([]*workerHandle, 0)
	for _, h := range s.workers {
		if !h.worker.IsExclusive() {
			nonExclusiveHandles = append(nonExclusiveHandles, h)
		}
	}
	s.mu.Unlock()

	// Broadcast CmdPause to all non-exclusive workers
	for _, h := range nonExclusiveHandles {
		select {
		case h.cmdChan <- CmdPause:
		case <-ctx.Done():
			return fmt.Errorf("context canceled while broadcasting CmdPause to %q: %w", h.worker.Name(), ctx.Err())
		}
	}

	// Wait for ACKs from all workers
	for _, h := range nonExclusiveHandles {
		select {
		case <-h.ackChan:
		case <-ctx.Done():
			return fmt.Errorf("context canceled while waiting for Ack from %q: %w", h.worker.Name(), ctx.Err())
		}
	}

	s.mu.Lock()
	s.paused = true
	s.mu.Unlock()

	return nil
}

// ResumeAll broadcasts CmdResume to all non-exclusive workers.
func (s *Scheduler) ResumeAll(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return errors.New("scheduler is not running")
	}
	if !s.paused {
		s.mu.Unlock()
		return nil
	}

	nonExclusiveHandles := make([]*workerHandle, 0)
	for _, h := range s.workers {
		if !h.worker.IsExclusive() {
			nonExclusiveHandles = append(nonExclusiveHandles, h)
		}
	}
	s.mu.Unlock()

	// Broadcast CmdResume to all paused workers
	for _, h := range nonExclusiveHandles {
		select {
		case h.cmdChan <- CmdResume:
		case <-ctx.Done():
			return fmt.Errorf("context canceled while broadcasting CmdResume to %q: %w", h.worker.Name(), ctx.Err())
		}
	}

	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()

	return nil
}

// ExecuteExclusive pauses all running non-exclusive probes, waits for ACKs, runs the provided exclusive task fn,
// and automatically resumes all paused probes upon completion.
func (s *Scheduler) ExecuteExclusive(ctx context.Context, fn func(ctx context.Context) error) error {
	s.exclusiveMu.Lock()
	defer s.exclusiveMu.Unlock()

	if err := s.PauseAll(ctx); err != nil {
		// Attempt resume in case of partial pause failure
		_ = s.ResumeAll(context.Background())
		return fmt.Errorf("failed to pause workers before exclusive task: %w", err)
	}

	// Ensure workers are resumed even if exclusive task errors or panics
	defer func() {
		resumeCtx, resumeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer resumeCancel()
		_ = s.ResumeAll(resumeCtx)
	}()

	return fn(ctx)
}

// Stop terminates all running workers gracefully and waits for their goroutines to exit.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.running = false
	s.paused = false
	s.mu.Unlock()

	s.wg.Wait()
}
