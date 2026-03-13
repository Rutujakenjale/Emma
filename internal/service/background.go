package service

import (
	"os"
	"strconv"
	"sync"
)

var bgWG sync.WaitGroup

// semaphore channel to limit concurrent background jobs
var bgSem chan struct{}

func init() {
	// default max concurrent background jobs
	max := 2
	if v := os.Getenv("MAX_CONCURRENT_JOBS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			max = n
		}
	}
	bgSem = make(chan struct{}, max)
}

// RunInBackground runs fn in a goroutine, tracks it in a waitgroup and
// respects the concurrency limit (bgSem). It blocks until a slot is available.
func RunInBackground(fn func()) {
	bgSem <- struct{}{}
	bgWG.Add(1)
	go func() {
		defer func() {
			<-bgSem
			bgWG.Done()
		}()
		fn()
	}()
}

// WaitBackground waits for all background jobs to finish.
func WaitBackground() {
	bgWG.Wait()
}
