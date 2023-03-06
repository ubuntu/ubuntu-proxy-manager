// Package lifecycle provides a streamlined way to manage the application
// lifecycle, queueing up runs and waiting for them to finish, returning all
// errors that occurred during the runs.
package lifecycle

import (
	"errors"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// ErrTimeoutReached is returned if the lifecycle times out.
var ErrTimeoutReached = errors.New("timeout reached")

// Lifecycle manages the application lifecycle.
type Lifecycle struct {
	timeout time.Duration
	counter sync.WaitGroup

	firstRun sync.Once
	quitMu   sync.RWMutex
	appMu    sync.Mutex

	errors []error

	exitRequested bool
	quitted       chan struct{}
	started       chan struct{}
}

// New returns a new Lifecycle with the given timeout.
func New(timeout time.Duration) *Lifecycle {
	return &Lifecycle{
		timeout: timeout,
		quitted: make(chan struct{}),
		started: make(chan struct{}),
	}
}

// Start starts or extends the lifecycle one run and blocks the next run until
// the current run finishes.
func (l *Lifecycle) Start() {
	l.increment()
	l.firstRun.Do(func() {
		// Signal that we can start waiting on the waitgroup and stop relying on the timeout
		close(l.started)
	})

	l.appMu.Lock()
}

// RunDone signals that a run has finished, allowing the next run to start.
func (l *Lifecycle) RunDone(err error) {
	l.errors = append(l.errors, err)
	l.decrement()
	l.appMu.Unlock()
}

// Wait waits for one of the following:
// - all runs to finish
// - the timeout to be reached
// - quit to be explicitly requested
// returning a joined representation of all errors that occurred during the runs.
func (l *Lifecycle) Wait() error {
	timer := time.NewTimer(l.timeout)

	for {
		select {
		case <-l.started:
			l.counter.Wait()
			return errors.Join(l.errors...)
		// Quit requested before the first run
		case <-l.quitted:
			return nil
		case <-timer.C:
			return ErrTimeoutReached
		}
	}
}

// Quit ends the lifecycle after the current run.
func (l *Lifecycle) Quit() {
	l.quitMu.Lock()
	defer l.quitMu.Unlock()

	close(l.quitted)
	l.exitRequested = true
}

// QuitRequested returns true if the application has been requested to quit.
func (l *Lifecycle) QuitRequested() bool {
	l.quitMu.RLock()
	defer l.quitMu.RUnlock()

	return l.exitRequested
}

func (l *Lifecycle) increment() {
	log.Debug("Lifecycle counter: +1")
	l.counter.Add(1)
}

func (l *Lifecycle) decrement() {
	log.Debug("Lifecycle counter: -1")
	l.counter.Done()
}
