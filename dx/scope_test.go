package treestamp_test

import (
	"context"
	"errors"
	"io/fs"
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

func TestWalkDirsSkipThisAndErrorCallback(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "keep.go"), "package keep\n")
	mustWriteFile(t, filepath.Join(root, "skip.go"), "package skip\n")
	var names []string
	boom := errors.New("boom")
	err := WalkDirs(root, DirWalkOptions{
		ScratchBuffer: NewScratchBuffer(),
		Callback: func(path string, d fs.DirEntry, err error) error {
			if err != nil || d == nil {
				return err
			}
			switch d.Name() {
			case "skip.go":
				return SkipThis
			case "keep.go":
				return boom
			}
			return nil
		},
		ErrorCallback: func(string, error) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	err = WalkDirs(root, DirWalkOptions{Callback: func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		if d.Name() != filepath.Base(root) {
			names = append(names, d.Name())
		}
		return nil
	}})
	if err != nil || len(names) != 2 {
		t.Fatalf("%v %v", names, err)
	}
}
