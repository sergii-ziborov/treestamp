package runtime

import (
	"context"
	"sync"
)

type Kind uint8

const (
	KindRoot Kind = iota
	KindDirectory
	KindMetadata
	KindContent
	kindCount
)

// Limits bound in-flight work and ready results. Zero means unlimited.
type Limits struct {
	Roots, Directories, Metadata, Content int
	Ready                                 int
	ReadyBytes                            int64
}

func DefaultLimits(workers int) Limits {
	if workers < 1 {
		workers = 1
	}
	return Limits{
		Roots: workers, Directories: workers, Metadata: workers, Content: workers,
		Ready: workers * 4, ReadyBytes: 32 << 20,
	}
}

// Budget is one admission pool for roots, directory work, metadata, and content.
// Ready results are limited by count and bytes, separately from goroutine slots.
type Budget struct {
	lim    Limits
	mu     sync.Mutex
	cond   *sync.Cond
	live   [kindCount]int
	readyN int
	readyB int64
	closed bool
}

func NewBudget(lim Limits) *Budget {
	b := &Budget{lim: lim}
	b.cond = sync.NewCond(&b.mu)
	return b
}

func (b *Budget) Close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.closed = true
	b.cond.Broadcast()
	b.mu.Unlock()
}

func (b *Budget) Admit(ctx context.Context, kind Kind) error {
	return b.wait(ctx, func() bool { return b.under(kind) }, func() { b.live[kind]++ })
}

func (b *Budget) Release(kind Kind) {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.live[kind] > 0 {
		b.live[kind]--
	}
	b.cond.Broadcast()
	b.mu.Unlock()
}

func (b *Budget) HoldReady(ctx context.Context, bytes int64) error {
	if bytes < 0 {
		bytes = 0
	}
	return b.wait(ctx, func() bool { return b.readyOK(bytes) }, func() {
		b.readyN++
		b.readyB += bytes
	})
}

func (b *Budget) DropReady(bytes int64) {
	if b == nil {
		return
	}
	if bytes < 0 {
		bytes = 0
	}
	b.mu.Lock()
	if b.readyN > 0 {
		b.readyN--
	}
	b.readyB -= bytes
	if b.readyB < 0 {
		b.readyB = 0
	}
	b.cond.Broadcast()
	b.mu.Unlock()
}

func (b *Budget) Live(kind Kind) int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	n := b.live[kind]
	b.mu.Unlock()
	return n
}

func (b *Budget) Ready() (n int, bytes int64) {
	if b == nil {
		return 0, 0
	}
	b.mu.Lock()
	n, bytes = b.readyN, b.readyB
	b.mu.Unlock()
	return n, bytes
}

func (b *Budget) wait(ctx context.Context, ok func() bool, take func()) error {
	if b == nil {
		return nil
	}
	stop := context.AfterFunc(ctx, func() {
		b.mu.Lock()
		b.cond.Broadcast()
		b.mu.Unlock()
	})
	defer stop()
	b.mu.Lock()
	defer b.mu.Unlock()
	for {
		if b.closed {
			return ErrBusy
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if ok() {
			take()
			return nil
		}
		b.cond.Wait()
	}
}

func (b *Budget) under(kind Kind) bool {
	max := b.maxOf(kind)
	return max <= 0 || b.live[kind] < max
}

func (b *Budget) readyOK(add int64) bool {
	if b.lim.Ready > 0 && b.readyN >= b.lim.Ready {
		return false
	}
	if b.lim.ReadyBytes > 0 && b.readyB+add > b.lim.ReadyBytes {
		return false
	}
	return true
}

func (b *Budget) maxOf(kind Kind) int {
	switch kind {
	case KindRoot:
		return b.lim.Roots
	case KindDirectory:
		return b.lim.Directories
	case KindMetadata:
		return b.lim.Metadata
	default:
		return b.lim.Content
	}
}
