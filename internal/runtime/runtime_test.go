package runtime

import (
	"errors"
	stdlib "runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenCancel(t *testing.T) {
	var tok Token
	if tok.Cancelled() {
		t.Fatal("fresh")
	}
	tok.Cancel()
	if !tok.Cancelled() {
		t.Fatal("cancelled")
	}
	(*Token)(nil).Cancel()
	if (*Token)(nil).Cancelled() {
		t.Fatal("nil")
	}
}

func TestDedicatedGlobalOwned(t *testing.T) {
	g := Global()
	if g.Parallelism() < 1 {
		t.Fatal("global")
	}
	d := Dedicated(2)
	if d.Parallelism() != 2 {
		t.Fatal(d.Parallelism())
	}
	var ran atomic.Bool
	d.Run(func() { ran.Store(true) })
	for i := 0; i < 50 && !ran.Load(); i++ {
		time.Sleep(time.Millisecond)
	}
	if !ran.Load() {
		t.Fatal("run")
	}
	busy := Dedicated(1)
	_ = busy.TryExecute(func() { time.Sleep(20 * time.Millisecond) })
	if err := busy.TryExecute(func() {}); err != ErrBusy {
		t.Fatalf("busy %v", err)
	}
	owned := Owned(goroutineExecutor{n: 3})
	if owned.Parallelism() != 3 {
		t.Fatal(owned.Parallelism())
	}
	if Owned(nil).Parallelism() < 1 {
		t.Fatal("nil owned")
	}
	grp := Dedicated(1).Group()
	grp.Go(func() error { return nil })
	if err := grp.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestDedicatedZeroUsesAvailable(t *testing.T) {
	if Dedicated(0).Parallelism() < 1 {
		t.Fatal("zero")
	}
	var zero Runtime
	if zero.Parallelism() < 1 {
		t.Fatal("zero rt")
	}
	var ran atomic.Bool
	if err := zero.TryExecute(func() { ran.Store(true) }); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50 && !ran.Load(); i++ {
		time.Sleep(time.Millisecond)
	}
	zero.Run(func() {})
	busy := Dedicated(1)
	_ = busy.TryExecute(func() { time.Sleep(30 * time.Millisecond) })
	grp := busy.Group()
	grp.Go(func() error { return errors.New("x") })
	grp.Go(func() error { return nil })
	if err := grp.Wait(); err == nil {
		t.Fatal("group err")
	}
}

func TestAdmitWaitsWithoutCallerRun(t *testing.T) {
	busy := Dedicated(1)
	hold := make(chan struct{})
	started := make(chan struct{})
	if err := busy.TryExecute(func() {
		close(started)
		<-hold
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	var ran atomic.Bool
	done := make(chan struct{})
	go func() {
		busy.Run(func() { ran.Store(true) })
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("caller-ran while the slot was held")
	case <-time.After(50 * time.Millisecond):
	}
	if ran.Load() {
		t.Fatal("caller-ran while the slot was held")
	}
	close(hold)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("admit hung")
	}
	for i := 0; i < 50 && !ran.Load(); i++ {
		time.Sleep(time.Millisecond)
	}
	if !ran.Load() {
		t.Fatal("admit")
	}
}

func TestAdmitTimeout(t *testing.T) {
	busy := Dedicated(1).WithAdmitTimeout(15 * time.Millisecond)
	started := make(chan struct{})
	_ = busy.TryExecute(func() {
		close(started)
		time.Sleep(80 * time.Millisecond)
	})
	<-started
	if err := busy.AdmitWait(func() {}); err != ErrAdmitTimeout {
		t.Fatalf("timeout %v", err)
	}
}

func TestGroupAdmitTimeoutUnblocks(t *testing.T) {
	busy := Dedicated(1).WithAdmitTimeout(10 * time.Millisecond)
	started := make(chan struct{})
	_ = busy.TryExecute(func() {
		close(started)
		time.Sleep(80 * time.Millisecond)
	})
	<-started
	g := busy.Group()
	g.Go(func() error { return nil })
	done := make(chan error, 1)
	go func() { done <- g.Wait() }()
	select {
	case err := <-done:
		if !errors.Is(err, ErrAdmitTimeout) {
			t.Fatalf("err %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("group wait hung after admit timeout")
	}
}

type closedExec struct{}

func (closedExec) TryExecute(Job) error { return errors.New("closed") }
func (closedExec) Parallelism() int     { return 1 }

func TestAdmitClosedExecutor(t *testing.T) {
	rt := Owned(closedExec{}).WithAdmitTimeout(50 * time.Millisecond)
	start := time.Now()
	err := rt.AdmitWait(func() {})
	if err == nil || errors.Is(err, ErrAdmitTimeout) {
		t.Fatalf("want closed, got %v", err)
	}
	if time.Since(start) > 30*time.Millisecond {
		t.Fatalf("spun on permanent reject: %v", time.Since(start))
	}
}

func TestOverflowCallerRuns(t *testing.T) {
	busy := Dedicated(1).OverflowCallerRuns()
	_ = busy.TryExecute(func() { time.Sleep(20 * time.Millisecond) })
	var ran atomic.Bool
	busy.Run(func() { ran.Store(true) })
	if !ran.Load() {
		t.Fatal("caller runs")
	}
}

func TestGroupDoesNotLeakAfterWait(t *testing.T) {
	before := stdlib.NumGoroutine()
	rt := Dedicated(2)
	grp := rt.Group()
	for i := 0; i < 4; i++ {
		grp.Go(func() error { return nil })
	}
	if err := grp.Wait(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for stdlib.NumGoroutine() > before+8 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if stdlib.NumGoroutine() > before+16 {
		t.Fatalf("goroutines %d before %d", stdlib.NumGoroutine(), before)
	}
}
