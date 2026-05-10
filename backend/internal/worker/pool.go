package worker

import (
	"errors"
	"sync"
)

var ErrStopped = errors.New("worker pool stopped")

type Pool struct {
	jobs    chan func()
	wg      sync.WaitGroup
	mu      sync.Mutex
	stopped bool
	stop    sync.Once
	done    chan struct{}
}

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
