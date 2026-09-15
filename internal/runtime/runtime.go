// Package runtime holds executors, admission, and bounded queues (P4).
package runtime

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
)

// Job is a unit of work. The executor must not store it after TryExecute
// returns; it either starts it or rejects it.
type Job func()

// Executor accepts a job once or rejects it.
type Executor interface {
	TryExecute(Job) error
	Parallelism() int
}

// Runtime is the shared worker budget used by walkers and scanners.
type Runtime struct {
	exec Executor
}

func Global() Runtime {
	return Runtime{exec: goroutineExecutor{n: available()}}
}

func Dedicated(n int) Runtime {
	if n <= 0 {
		n = available()
	}
	return Runtime{exec: newPool(n)}
}

func Owned(exec Executor) Runtime {
	if exec == nil {
		return Global()
	}
	return Runtime{exec: exec}
}

func (r Runtime) Parallelism() int {
	if r.exec == nil {
		return available()
	}
	return r.exec.Parallelism()
}

func (r Runtime) TryExecute(job Job) error {
	if r.exec == nil {
		return Global().TryExecute(job)
	}
	return r.exec.TryExecute(job)
}

func (r Runtime) Run(job Job) {
	if err := r.TryExecute(job); err != nil {
		job()
	}
}

type goroutineExecutor struct{ n int }

func (g goroutineExecutor) Parallelism() int { return g.n }

func (g goroutineExecutor) TryExecute(job Job) error {
	go job()
	return nil
}

type poolExecutor struct {
	n     int
	slots chan struct{}
}

func newPool(n int) *poolExecutor {
	return &poolExecutor{n: n, slots: make(chan struct{}, n)}
}

func (p *poolExecutor) Parallelism() int { return p.n }

func (p *poolExecutor) TryExecute(job Job) error {
	select {
	case p.slots <- struct{}{}:
		go func() {
			defer func() { <-p.slots }()
			job()
		}()
		return nil
	default:
		return ErrBusy
	}
}

// Group runs a bounded set of jobs and waits.
type Group struct {
	rt  Runtime
	wg  sync.WaitGroup
	mu  sync.Mutex
	err error
}

func (r Runtime) Group() *Group { return &Group{rt: r} }

func (g *Group) Go(job func() error) {
	g.wg.Add(1)
	wrapped := func() {
		defer g.wg.Done()
		if err := job(); err != nil {
			g.mu.Lock()
			if g.err == nil {
				g.err = err
			}
			g.mu.Unlock()
		}
	}
	if err := g.rt.TryExecute(wrapped); err != nil {
		wrapped()
	}
}

func (g *Group) Wait() error {
	g.wg.Wait()
	return g.err
}

func available() int {
	n := runtime.GOMAXPROCS(0)
	if n < 1 {
		return 1
	}
	return n
}

// Token is a cheap cooperative cancel flag.
type Token struct{ cancelled atomic.Bool }

func (t *Token) Cancel() {
	if t != nil {
		t.cancelled.Store(true)
	}
}

func (t *Token) Cancelled() bool {
	return t != nil && t.cancelled.Load()
}

var ErrBusy = errors.New("executor rejected job")
