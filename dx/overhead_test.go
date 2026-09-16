package treestamp_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sergii-ziborov/treestamp"
)

func BenchmarkFacadeVsScan(b *testing.B) {
	root := b.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n"), 0o644); err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.Run("Scan", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := treestamp.Scan(ctx, root); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("ScanWith", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := treestamp.ScanWith(ctx, root); err != nil {
				b.Fatal(err)
			}
		}
	})
}
