package official_test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sergii-ziborov/treestamp"
)

func TestOfficialCampaign(t *testing.T) {
	if os.Getenv("TREESTAMP_OFFICIAL") == "" {
		t.Skip("set TREESTAMP_OFFICIAL=1 to record the official campaign")
	}
	n := 1000
	root := t.TempDir()
	for i := 0; i < n; i++ {
		dir := filepath.Join(root, fmt.Sprintf("g%02d", i%20))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(dir, fmt.Sprintf("f%03d.go", i))
		if err := os.WriteFile(name, []byte("package f\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	measure(t, "B01", func() error {
		return treestamp.Walk(root, func(string, fs.DirEntry, error) error { return nil })
	})
	measure(t, "B02", func() error {
		return treestamp.WalkWithConfig(root, treestamp.Config{Sort: true}, func(string, fs.DirEntry, error) error { return nil })
	})
	measure(t, "B03", func() error {
		_, err := treestamp.ScanPathsWith(ctx, root)
		return err
	})
	measure(t, "B04", func() error {
		_, err := treestamp.ScanPathsWith(ctx, root, treestamp.WithExtensions("go"))
		return err
	})
	measure(t, "B05", func() error {
		_, err := treestamp.ScanPathsWith(ctx, root, treestamp.WithExtensions("go"))
		return err
	})
	measure(t, "B06", func() error {
		_, err := treestamp.ScanWith(ctx, root, treestamp.WithExtensions("go"))
		return err
	})
	measure(t, "B07", func() error {
		_, err := treestamp.ScanWith(ctx, root, treestamp.WithExtensions("go"))
		return err
	})
	measure(t, "B08", func() error {
		_, err := treestamp.EachFile(ctx, root, func(treestamp.ScannedFile, []byte) error { return nil }, treestamp.WithExtensions("go"))
		return err
	})
	var cache treestamp.ScanCache
	measure(t, "B09", func() error {
		rep, err := treestamp.ScanWith(ctx, root, treestamp.WithExtensions("go"))
		if err != nil {
			return err
		}
		cache = rep.ToCache()
		scanner, err := treestamp.NewScanner(root, treestamp.WithOptions(treestamp.DefaultOptions().WithExtensions("go")))
		if err != nil {
			return err
		}
		_, err = scanner.ScanCached(ctx, &cache)
		return err
	})
	measure(t, "B10", func() error {
		scanner, err := treestamp.NewScanner(root, treestamp.WithOptions(treestamp.DefaultOptions().WithExtensions("go")))
		if err != nil {
			return err
		}
		_, err = scanner.ScanCached(ctx, &cache)
		return err
	})
	measure(t, "B11", func() error {
		_, err := treestamp.NewMultiScanner(root).Scan(ctx)
		return err
	})
	measure(t, "B12", func() error {
		m, err := treestamp.NewSelectionMatcher(root, treestamp.DefaultOptions())
		if err != nil {
			return err
		}
		_ = m.DecidePath(treestamp.PathQuery{Rel: "g00/f000.go", Name: "f000.go", IsFile: true})
		return nil
	})
	measure(t, "B13", func() error {
		_, err := treestamp.ScanWith(ctx, root, treestamp.WithExtensions("go"), treestamp.WithAdmitTimeout(time.Second))
		return err
	})
	measure(t, "B14", func() error {
		_, err := treestamp.Explain(root, "g00/f000.go")
		return err
	})
}

func measure(t *testing.T, id string, fn func() error) {
	t.Helper()
	start := time.Now()
	if err := fn(); err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	fmt.Printf("OFFICIAL %s MEASURED ns=%d\n", id, time.Since(start).Nanoseconds())
}
