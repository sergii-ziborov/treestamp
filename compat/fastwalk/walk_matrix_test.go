package fastwalk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWalkRelativeRootKeepsRelativePaths(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "a")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	var paths []string
	err = Walk(&Config{NumWorkers: 1}, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no paths")
	}
	for _, p := range paths {
		if filepath.IsAbs(p) {
			t.Fatalf("absolute path %q", p)
		}
	}
}

func TestWalkWorkersAndLexicalSort(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"c.txt", "a.txt", "b.txt"} {
		mustWrite(t, filepath.Join(root, name), "x")
	}
	for _, workers := range []int{0, 1, 2} {
		var names []string
		err := Walk(&Config{NumWorkers: workers, Sort: SortLexical}, root, func(_ string, d fs.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return err
			}
			names = append(names, d.Name())
			return nil
		})
		if err != nil || strings.Join(names, ",") != "a.txt,b.txt,c.txt" {
			t.Fatalf("workers=%d names=%q err=%v", workers, names, err)
		}
	}
}

func TestWalkSkipAllStaysError(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	err := Walk(&Config{NumWorkers: 2}, root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		return fs.SkipAll
	})
	if !errors.Is(err, fs.SkipAll) {
		t.Fatalf("SkipAll %v", err)
	}
}

func TestWalkTraverseLinkOutsideWhenFollowFalse(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "out.txt"), "x")
	link := filepath.Join(root, "out")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip(err)
	}
	var seen []string
	err := Walk(&Config{Follow: false, NumWorkers: 1}, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		if path == link {
			return ErrTraverseLink
		}
		seen = append(seen, filepath.Base(path))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range seen {
		if name == "out.txt" {
			return
		}
	}
	t.Fatalf("outside target missing: %v", seen)
}
