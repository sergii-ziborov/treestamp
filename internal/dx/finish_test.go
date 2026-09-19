package dx

import (
	"context"
	"errors"
	"testing"
)

func TestCheckPartialAndCancel(t *testing.T) {
	if err := Check(context.Background(), Outcome{Complete: true}); err != nil {
		t.Fatal(err)
	}
	if err := Check(context.Background(), Outcome{Unread: true}); !errors.Is(err, ErrPartial) {
		t.Fatalf("%v", err)
	}
	if err := Check(context.Background(), Outcome{Term: TermMaxEntries}); !errors.Is(err, ErrPartial) {
		t.Fatalf("limit %v", err)
	}
	if err := Finish(context.Background(), Outcome{Term: TermMaxEntries}, true); err != nil {
		t.Fatalf("stop %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Check(ctx, Outcome{Complete: true}); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
	if Classify(context.DeadlineExceeded) != "timeout" {
		t.Fatal(Classify(context.DeadlineExceeded))
	}
}
