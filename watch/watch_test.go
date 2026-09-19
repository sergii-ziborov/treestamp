package watch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCloseThenPlanIsErrClosed(t *testing.T) {
	root := t.TempDir()
	w, err := Open(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = w.Plan(context.Background())
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("got %v", err)
	}
}

func TestOpenWatchesNestedDirectory(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := Open(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := w.Plan(ctx)
		done <- err
	}()
	time.Sleep(80 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(nested, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
