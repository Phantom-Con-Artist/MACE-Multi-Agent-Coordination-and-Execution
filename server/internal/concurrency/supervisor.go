package concurrency

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Service defines a component with lifecycle hooks managed by a supervisor.
type Service interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// SupervisorGroup manages the concurrent execution and shutdown of multiple services.
type SupervisorGroup struct {
	services []Service
	mu       sync.Mutex
	cancel   context.CancelFunc
	errChan  chan error
	wg       sync.WaitGroup
}

// NewSupervisorGroup creates a supervisor group.
func NewSupervisorGroup() *SupervisorGroup {
	return &SupervisorGroup{
		errChan: make(chan error, 16),
	}
}

// Add registers a service with the supervisor.
func (sg *SupervisorGroup) Add(svc Service) {
	sg.mu.Lock()
	defer sg.mu.Unlock()
	sg.services = append(sg.services, svc)
}

// Start launches all registered services concurrently.
func (sg *SupervisorGroup) Start(parentCtx context.Context) error {
	ctx, cancel := context.WithCancel(parentCtx)
	sg.cancel = cancel

	for _, svc := range sg.services {
		s := svc
		sg.wg.Add(1)
		go func() {
			defer sg.wg.Done()
			if err := s.Start(ctx); err != nil && err != context.Canceled {
				select {
				case sg.errChan <- fmt.Errorf("service %s failed: %w", s.Name(), err):
				default:
				}
				// Trigger shutdown of sibling services on error
				cancel()
			}
		}()
	}

	return nil
}

// Stop gracefully terminates all services within a deadline.
func (sg *SupervisorGroup) Stop(timeout time.Duration) {
	if sg.cancel != nil {
		sg.cancel()
	}

	done := make(chan struct{})
	go func() {
		// Stop in reverse registration order
		sg.mu.Lock()
		defer sg.mu.Unlock()
		for i := len(sg.services) - 1; i >= 0; i-- {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), timeout)
			_ = sg.services[i].Stop(stopCtx)
			stopCancel()
		}
		sg.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// Errors returns the channel on which unexpected service failures are delivered.
func (sg *SupervisorGroup) Errors() <-chan error {
	return sg.errChan
}
