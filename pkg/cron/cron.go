package cron

import (
	"context"
	"log"
	"sync"
	"time"
)

// Job is a function that performs a task. It accepts a context which
// should be respected for cancellation.
type Job func(ctx context.Context) error

// Manager manages scheduled jobs.
type Manager struct {
	mu      sync.Mutex
	jobs    []*scheduledJob
	running bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

type scheduledJob struct {
	name     string
	interval time.Duration
	fn       Job
}

// NewManager creates a new job manager.
func NewManager() *Manager {
	return &Manager{
		stopCh: make(chan struct{}),
	}
}

// Add registers a new job with the given name and interval.
// If the manager is already running, the job will not start until the manager is restarted
// (or this implementation could be enhanced to start immediately, but for now we assume configuration before start).
func (m *Manager) Add(name string, interval time.Duration, job Job) {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Printf("Adding cron job: %s, interval: %s", name, interval)

	m.jobs = append(m.jobs, &scheduledJob{
		name:     name,
		interval: interval,
		fn:       job,
	})
	
	if m.running {
		// If adding while running, we should start this job immediately.
		m.wg.Add(1)
		go m.runJob(job, name, interval, m.stopCh)
	}
}

// Start begins executing all registered jobs.
func (m *Manager) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return
	}

	m.running = true
	// Re-make stopCh in case it was closed in Stop()
	select {
	case <-m.stopCh:
		m.stopCh = make(chan struct{})
	default:
	}

	for _, j := range m.jobs {
		m.wg.Add(1)
		go m.runJob(j.fn, j.name, j.interval, m.stopCh)
	}
	
	log.Println("Cron manager started")
}

// Stop stops all running jobs and waits for them to finish.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	close(m.stopCh)
	m.wg.Wait()
	m.running = false
	log.Println("Cron manager stopped")
}

func (m *Manager) runJob(job Job, name string, interval time.Duration, stopCh <-chan struct{}) {
	defer m.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("Cron job %s panicked: %v", name, r)
					}
				}()

				// Create a context for the individual job run?
				// Or just pass a background context?
				// If we want the job to be cancelable via Stop(), we might want to pass a context derived from stopCh logic,
				// but simplistic is fine.
				ctx := context.Background()
				
				if err := job(ctx); err != nil {
					log.Printf("Cron job %s failed: %v", name, err)
				} else {
					// Optional: Log success or debug
					// log.Printf("Cron job %s completed successfully", name)
				}
			}()
		}
	}
}

// RunOnce runs the given job immediately. Useful for testing or manual triggers.
func RunOnce(ctx context.Context, job Job) error {
	return job(ctx)
}
