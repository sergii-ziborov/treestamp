package runtime

import (
	"errors"
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
