package concurrency

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestPool_Execution(t *testing.T) {
	ctx := context.Background()
	pool := NewPool(ctx, 3, 10)
	defer pool.Stop()

	var counter int64
	for i := 0; i < 10; i++ {
		err := pool.Submit(func(ctx context.Context) error {
			atomic.AddInt64(&counter, 1)
			return nil
		})
		if err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
	}

	// Give workers time to process
	time.Sleep(50 * time.Millisecond)
	pool.Stop()

	if val := atomic.LoadInt64(&counter); val != 10 {
		t.Errorf("expected counter 10, got %d", val)
	}

	// After stop, submit should return ErrPoolClosed
	err := pool.Submit(func(ctx context.Context) error { return nil })
	if !errors.Is(err, ErrPoolClosed) {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}
}

type mockService struct {
	name    string
	started int64
	stopped int64
	fail    bool
}

func (m *mockService) Name() string { return m.name }
func (m *mockService) Start(ctx context.Context) error {
	atomic.StoreInt64(&m.started, 1)
	if m.fail {
		return errors.New("boom")
	}
	<-ctx.Done()
	return nil
}
func (m *mockService) Stop(ctx context.Context) error {
	atomic.StoreInt64(&m.stopped, 1)
	return nil
}

func TestSupervisorGroup_Lifecycle(t *testing.T) {
	sg := NewSupervisorGroup()
	svc1 := &mockService{name: "svc1"}
	svc2 := &mockService{name: "svc2"}

	sg.Add(svc1)
	sg.Add(svc2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sg.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	if atomic.LoadInt64(&svc1.started) != 1 || atomic.LoadInt64(&svc2.started) != 1 {
		t.Error("expected both services to start")
	}

	sg.Stop(1 * time.Second)
	if atomic.LoadInt64(&svc1.stopped) != 1 || atomic.LoadInt64(&svc2.stopped) != 1 {
		t.Error("expected both services to be stopped")
	}
}

func TestSupervisorGroup_FailureCascade(t *testing.T) {
	sg := NewSupervisorGroup()
	failingSvc := &mockService{name: "failing", fail: true}
	healthySvc := &mockService{name: "healthy"}

	sg.Add(failingSvc)
	sg.Add(healthySvc)

	ctx := context.Background()
	_ = sg.Start(ctx)

	select {
	case err := <-sg.Errors():
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for failure error")
	}

	sg.Stop(500 * time.Millisecond)
}
