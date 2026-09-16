package runtime

import (
	"context"
	"testing"
	"time"
)

func TestBudgetAdmitsAndBoundsReady(t *testing.T) {
	b := NewBudget(Limits{Directories: 1, Ready: 1, ReadyBytes: 8})
	ctx := context.Background()
	if err := b.Admit(ctx, KindDirectory); err != nil {
		t.Fatal(err)
	}
	if b.Live(KindDirectory) != 1 {
		t.Fatal(b.Live(KindDirectory))
	}
	if err := b.HoldReady(ctx, 4); err != nil {
		t.Fatal(err)
	}
	n, bytes := b.Ready()
	if n != 1 || bytes != 4 {
		t.Fatalf("%d %d", n, bytes)
	}
	blocked, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if err := b.Admit(blocked, KindDirectory); err == nil {
		t.Fatal("second directory slot")
	}
	full, cancel2 := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel2()
	if err := b.HoldReady(full, 8); err == nil {
		t.Fatal("ready bytes")
	}
	b.Release(KindDirectory)
	b.DropReady(4)
	if err := b.Admit(ctx, KindDirectory); err != nil {
		t.Fatal(err)
	}
	b.Release(KindDirectory)
	b.Close()
	if err := b.Admit(ctx, KindDirectory); err != ErrBusy {
		t.Fatalf("closed %v", err)
	}
}

func TestBudgetUnlimitedZero(t *testing.T) {
	b := NewBudget(Limits{})
	ctx := context.Background()
	for i := 0; i < 8; i++ {
		if err := b.Admit(ctx, KindContent); err != nil {
			t.Fatal(err)
		}
	}
	if b.Live(KindContent) != 8 {
		t.Fatal(b.Live(KindContent))
	}
}
