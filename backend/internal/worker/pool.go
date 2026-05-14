// Package worker provides a simple goroutine pool for executing asynchronous jobs.
package worker

import (
	"errors"
	"sync"
)

// ErrStopped is returned when attempting to submit a job to a stopped pool.
var ErrStopped = errors.New("worker pool stopped")

// Pool is a fixed-size goroutine pool that accepts functions for asynchronous execution.
// Callers submit jobs with Submit and shut down with Stop. Stop waits for all
// in-flight jobs to complete.
type Pool struct {
	jobs    chan func()
	wg      sync.WaitGroup
	mu      sync.Mutex
	stopped bool
	stop    sync.Once
	done    chan struct{}
}

// New creates a pool with the given number of workers. If size <= 0, 1 is used.
func New(size int) *Pool {
	if size <= 0 {
		size = 1
	}
	p := &Pool{
		jobs: make(chan func(), size),
		done: make(chan struct{}),
	}
	for i := 0; i < size; i++ {
		go func() {
			for job := range p.jobs {
				func() {
					defer p.wg.Done()
					job()
				}()
			}
		}()
	}
	return p
}

// Submit adds a job to the pool. It returns ErrStopped if the pool has been stopped.
func (p *Pool) Submit(job func()) error {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return ErrStopped
	}
	p.wg.Add(1)
	p.mu.Unlock()
	p.jobs <- job
	return nil
}

// Stop initiates graceful shutdown: it prevents new submissions, waits for
// all running jobs to finish, then closes internal channels.
func (p *Pool) Stop() {
	p.stop.Do(func() {
		p.mu.Lock()
		p.stopped = true
		p.mu.Unlock()
		p.wg.Wait()
		close(p.jobs)
		close(p.done)
	})
	<-p.done
}
