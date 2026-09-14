package treestamp

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestScanAPINotImplemented(t *testing.T) {
	ctx := context.Background()
	if _, err := Scan(ctx, "."); !IsNotImplemented(err) {
		t.Fatalf("Scan: %v", err)
	}
	if _, err := ScanCompact(ctx, "."); !IsNotImplemented(err) {
		t.Fatalf("ScanCompact: %v", err)
	}
	if _, err := ScanPaths(ctx, "."); !IsNotImplemented(err) {
		t.Fatalf("ScanPaths: %v", err)
	}
	if _, err := NewScanner("."); !IsNotImplemented(err) {
		t.Fatalf("NewScanner: %v", err)
	}
}

func TestDefaultOptionsDoNotAddTreestampIgnore(t *testing.T) {
	opts := DefaultOptions()
	for _, name := range opts.IgnoreFiles {
		if name == ".treestampignore" {
			t.Fatal("compatible preset must not add .treestampignore")
		}
	}
	if opts.MaxFileBytes != 1_500_000 {
		t.Fatalf("max_file_bytes=%d", opts.MaxFileBytes)
	}
	if opts.SkipHidden {
		t.Fatal("skip_hidden default is false")
	}
}

func TestPublicWalker(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	walker, err := NewWalker(root)
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	found := false
	for {
		entry, err := walker.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if entry.IsFile() && entry.FileName() == "x.txt" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing x.txt")
	}
}

func TestPathHelpers(t *testing.T) {
	if !IsSameOrDescendant("src/a.go", "src") {
		t.Fatal("descendant")
	}
	got := CollapsePathPrefixes([]string{"src/nested", "src"})
	if len(got) != 1 || got[0] != "src" {
		t.Fatalf("collapse=%v", got)
	}
}
