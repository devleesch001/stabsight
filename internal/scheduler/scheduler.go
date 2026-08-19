package scheduler

import (
	"errors"
	"fmt"
	"sync"
)

type workerHandle struct {
	worker  ProbeWorker
	cmdChan chan Command
	ackChan chan struct{}
}

// Scheduler coordinates the lifecycle, concurrency, and exclusive execution of network probe workers.
type Scheduler struct {
	mu      sync.RWMutex
	workers []*workerHandle
	running bool
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
