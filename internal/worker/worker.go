package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"coupon-import/internal/metrics"
	"coupon-import/internal/service"
)

// Job represents a background import job to process.
type Job struct {
	ID   string
	Path string
}

// Pool processes jobs with a bounded number of workers.
type Pool struct {
	svc    service.ImportServiceInterface
	jobs   chan Job
	wg     sync.WaitGroup
	closed chan struct{}
}

// NewPool creates a pool with the given capacity and worker count.
func NewPool(svc service.ImportServiceInterface, capacity, workers int) *Pool {
	p := &Pool{
		svc:    svc,
		jobs:   make(chan Job, capacity),
		closed: make(chan struct{}),
	}
	p.start(workers)
	metrics.WorkerCount.Set(float64(workers))
	return p
}

func (p *Pool) start(n int) {
	for i := 0; i < n; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for j := range p.jobs {
				// update queue length metric after receiving job
				metrics.QueueLength.Set(float64(len(p.jobs)))
				if err := p.svc.ProcessFile(j.ID, j.Path); err != nil {
					metrics.JobFailed.Inc()
					log.Printf("worker: process error job=%s: %v", j.ID, err)
				} else {
					metrics.JobProcessed.Inc()
				}
				// update queue length again
				metrics.QueueLength.Set(float64(len(p.jobs)))
			}
		}()
	}
}

// Enqueue attempts to queue a job and returns false if the pool is closed.
func (p *Pool) Enqueue(j Job) bool {
	select {
	case <-p.closed:
		return false
	default:
	}
	select {
	case p.jobs <- j:
		metrics.QueueLength.Set(float64(len(p.jobs)))
		metrics.JobAccepted.Inc()
		return true
	default:
		// queue is full, do a blocking send with timeout to avoid forever blocking
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case p.jobs <- j:
			metrics.QueueLength.Set(float64(len(p.jobs)))
			metrics.JobAccepted.Inc()
			return true
		case <-timer.C:
			return false
		}
	}
}

// Shutdown closes the queue and waits for workers to finish. Context may cancel waiting.
func (p *Pool) Shutdown(ctx context.Context) error {
	close(p.closed)
	close(p.jobs)
	metrics.WorkerCount.Set(0)
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
