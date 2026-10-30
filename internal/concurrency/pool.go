package concurrency

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrPoolClosed = errors.New("worker pool is closed")
)

// Task is a unit of work executed by the pool.
type Task func(ctx context.Context) error

// Pool executes tasks concurrently up to a specified limit.
type Pool struct {
	maxWorkers int
	tasks      chan Task
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	closed     bool
	mu         sync.Mutex
}

// NewPool creates and starts a worker pool with maxWorkers goroutines.
func NewPool(ctx context.Context, maxWorkers int, queueSize int) *Pool {
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	if queueSize <= 0 {
		queueSize = maxWorkers * 2
	}

	pCtx, cancel := context.WithCancel(ctx)
	p := &Pool{
		maxWorkers: maxWorkers,
		tasks:      make(chan Task, queueSize),
		ctx:        pCtx,
		cancel:     cancel,
	}

	for i := 0; i < maxWorkers; i++ {
		p.wg.Add(1)
		go p.worker()
	}

	return p
}

func (p *Pool) worker() {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case task, ok := <-p.tasks:
			if !ok {
				return
			}
			if task != nil {
				_ = task(p.ctx)
			}
		}
	}
}

// Submit queues a task for execution. Returns ErrPoolClosed if closed.
func (p *Pool) Submit(task Task) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrPoolClosed
	}

	select {
	case <-p.ctx.Done():
		return p.ctx.Err()
	case p.tasks <- task:
		return nil
	}
}

// Stop stops accepting new tasks and waits for in-flight tasks to complete.
func (p *Pool) Stop() {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.tasks)
	}
	p.mu.Unlock()

	p.wg.Wait()
	p.cancel()
}
