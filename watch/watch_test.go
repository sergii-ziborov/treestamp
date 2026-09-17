package watch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenAndCancel(t *testing.T) {
	root := t.TempDir()
	w, err := Open(root, []string{".gitignore"})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := w.Plan(ctx); err == nil {
		t.Fatal("expected cancel")
	}
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := w.Plan(ctx); err != nil {
		t.Fatal(err)
	}
}
