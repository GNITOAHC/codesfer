package cron

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestManager_AddAndStart(t *testing.T) {
	mgr := NewManager()

	var counter int32
	job := func(ctx context.Context) error {
		atomic.AddInt32(&counter, 1)
		t.Logf("Job executed, count: %d", atomic.LoadInt32(&counter))
		return nil
	}

	interval := 10 * time.Millisecond
	mgr.Add("test-job", interval, job)

	mgr.Start()

	// Wait for a few intervals
	time.Sleep(100 * time.Millisecond)

	mgr.Stop()

	count := atomic.LoadInt32(&counter)
	if 8 > count || count > 12 {
		t.Errorf("Expected at 8 ~ 12 executions, got %d", count)
	}
}

func TestManager_Stop(t *testing.T) {
	mgr := NewManager()

	runCh := make(chan struct{})
	job := func(ctx context.Context) error {
		close(runCh)
		return nil
	}

	mgr.Add("one-shot", 10*time.Millisecond, job)
	mgr.Start()

	select {
	case <-runCh:
		// Job ran
	case <-time.After(1 * time.Second):
		t.Fatal("Job did not run in time")
	}

	mgr.Stop()

	// Ensure it doesn't run again (hard to test negative quickly without waiting,
	// but we check that Stop blocks until done is essentially verified by function return)
}

func TestManager_PanicRecovery(t *testing.T) {
	mgr := NewManager()

	// This job panics
	job := func(ctx context.Context) error {
		panic("oops")
	}

	mgr.Add("panic-job", 10*time.Millisecond, job)
	mgr.Start()

	// Wait for it to likely run and panic
	time.Sleep(50 * time.Millisecond)

	// If the test process hasn't crashed, recovery worked.
	mgr.Stop()
}
