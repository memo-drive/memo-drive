package worker

import "sync"

type Pool struct {
	jobs chan func()
	wg   sync.WaitGroup
}

func New(size int) *Pool {
	if size <= 0 {
		size = 1
	}
	p := &Pool{jobs: make(chan func())}
	for i := 0; i < size; i++ {
		go func() {
			for job := range p.jobs {
				job()
				p.wg.Done()
			}
		}()
	}
	return p
}

func (p *Pool) Submit(job func()) {
	p.wg.Add(1)
	p.jobs <- job
}

func (p *Pool) Stop() {
	p.wg.Wait()
	close(p.jobs)
}
