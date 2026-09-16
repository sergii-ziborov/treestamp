package treestamp_test

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/sergii-ziborov/treestamp"
)

func TestWithScopeDoesNotLiftIgnore(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "keep"))
	mustMkdir(t, filepath.Join(root, "other"))
	mustWriteFile(t, filepath.Join(root, ".gitignore"), "secret.go\n")
	mustWriteFile(t, filepath.Join(root, "keep", "ok.go"), "package keep\n")
	mustWriteFile(t, filepath.Join(root, "keep", "secret.go"), "package secret\n")
	mustWriteFile(t, filepath.Join(root, "other", "no.go"), "package other\n")
	paths, err := ScanPathsWith(context.Background(), root, WithScope("keep/**"))
	if err != nil {
		t.Fatal(err)
	}
	if !containsPath(paths, "ok.go") || containsPath(paths, "secret.go") || containsPath(paths, "no.go") {
		t.Fatalf("%v", paths)
	}
}
