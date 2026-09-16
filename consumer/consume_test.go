package consumer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sergii-ziborov/treestamp"
)

func TestPublicFacadeCompiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := treestamp.ScanWith(context.Background(), root, treestamp.WithExtensions("go"))
	if err != nil || report == nil || len(report.Files) != 1 {
		t.Fatalf("%v %v", report, err)
	}
}
