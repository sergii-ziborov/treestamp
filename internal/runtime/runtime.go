// Package runtime holds executors, admission, and bounded queues (P4).
package runtime

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
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
	exec         Executor
	callerRuns   bool
	admitTimeout time.Duration
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

// OverflowCallerRuns runs a rejected job on the caller. Opt-in only.
func (r Runtime) OverflowCallerRuns() Runtime {
	r.callerRuns = true
	return r
}

// WithAdmitTimeout bounds how long Admit waits for a worker slot.
func (r Runtime) WithAdmitTimeout(d time.Duration) Runtime {
	r.admitTimeout = d
	return r
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

func (r Runtime) Run(job Job) error {
	if r.callerRuns {
		if err := r.TryExecute(job); err != nil {
			job()
		}
		return nil
	}
	return r.AdmitWait(job)
}

// Admit starts job under the executor, waiting for a slot. It never
// silently runs the job on the caller.
func (r Runtime) Admit(job Job) error {
	return r.AdmitWait(job)
}

// AdmitWait starts job under the executor. A configured timeout
// returns ErrAdmitTimeout instead of waiting forever.
func (r Runtime) AdmitWait(job Job) error {
	exec := r.exec
	if exec == nil {
		exec = Global().exec
	}
	if w, ok := exec.(interface {
		AdmitWait(Job, time.Duration) error
	}); ok {
		return w.AdmitWait(job, r.admitTimeout)
	}
	deadline := time.Time{}
	if r.admitTimeout > 0 {
		deadline = time.Now().Add(r.admitTimeout)
	}
	for {
		err := exec.TryExecute(job)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrBusy) {
			return err
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return ErrAdmitTimeout
		}
		runtime.Gosched()
	}
}

type goroutineExecutor struct{ n int }

func (g goroutineExecutor) Parallelism() int { return g.n }

func (g goroutineExecutor) TryExecute(job Job) error {
	go job()
	return nil
}

func (g goroutineExecutor) Admit(job Job) { go job() }

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

func (p *poolExecutor) Admit(job Job) error {
	return p.AdmitWait(job, 0)
}

func (p *poolExecutor) AdmitWait(job Job, d time.Duration) error {
	if d <= 0 {
		p.slots <- struct{}{}
		p.start(job)
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case p.slots <- struct{}{}:
		p.start(job)
		return nil
	case <-timer.C:
		return ErrAdmitTimeout
	}
}

func (p *poolExecutor) start(job Job) {
	go func() {
		defer func() { <-p.slots }()
		job()
	}()
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
			g.setErr(err)
		}
	}
	if g.rt.callerRuns {
		if err := g.rt.TryExecute(wrapped); err != nil {
			wrapped()
		}
		return
	}
	if err := g.rt.AdmitWait(wrapped); err != nil {
		g.wg.Done()
		g.setErr(err)
	}
}

func (g *Group) setErr(err error) {
	g.mu.Lock()
	if g.err == nil {
		g.err = err
	}
	g.mu.Unlock()
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

var (
	ErrBusy         = errors.New("executor rejected job")
	ErrAdmitTimeout = errors.New("executor admission timed out")
)
