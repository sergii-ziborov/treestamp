package godirwalk

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWalkImportReplacement(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "keep.txt"), "k")
	mustWrite(t, filepath.Join(root, "skip.txt"), "s")
	mustWrite(t, filepath.Join(root, "gone", "x.txt"), "x")
	var names []string
	err := Walk(root, &Options{
		Callback: func(path string, de *Dirent) error {
			base := de.Name()
			if base == "skip.txt" || base == "gone" {
				return SkipThis
			}
			if base != filepath.Base(root) {
				names = append(names, base)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "keep.txt" {
		t.Fatalf("%q", names)
	}
}

func TestWalkErrorCallbackSkip(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	boom := errors.New("boom")
	var saw string
	err := Walk(root, &Options{
		Callback: func(path string, de *Dirent) error {
			if de.Name() == "a.txt" {
				return boom
			}
			return nil
		},
		ErrorCallback: func(path string, err error) ErrorAction {
			saw = filepath.Base(path)
			if errors.Is(err, boom) {
				return SkipNode
			}
			return Halt
		},
	})
	if err != nil || saw != "a.txt" {
		t.Fatalf("%v %q", err, saw)
	}
}

func TestWalkRejectsFileRoot(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "only.txt")
	mustWrite(t, file, "x")
	err := Walk(file, &Options{Callback: func(string, *Dirent) error { return nil }})
	if err == nil || !strings.Contains(err.Error(), "cannot Walk non-directory") {
		t.Fatalf("%v", err)
	}
	err = Walk(file, &Options{AllowNonDirectory: true, Callback: func(string, *Dirent) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReadDirentsAndScanner(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "z.txt"), "z")
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	scratch := make([]byte, MinimumScratchBufferSize)
	ents, err := ReadDirents(root, scratch)
	if err != nil || len(ents) != 2 {
		t.Fatalf("%v %v", ents, err)
	}
	names, err := ReadDirnames(root, scratch)
	if err != nil || len(names) != 2 {
		t.Fatalf("%v %v", names, err)
	}
	sc, err := NewScannerWithScratchBuffer(root, scratch)
	if err != nil {
		t.Fatal(err)
	}
	var got int
	for sc.Scan() {
		de, err := sc.Dirent()
		if err != nil || de.Name() == "" {
			t.Fatal(err)
		}
		got++
	}
	if err := sc.Err(); err != nil || got != 2 {
		t.Fatalf("%d %v", got, err)
	}
}

func TestWalkRelativeRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "tree")
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(parent); err != nil {
		t.Fatal(err)
	}
	var paths []string
	err = Walk("tree", &Options{Callback: func(path string, _ *Dirent) error {
		paths = append(paths, filepath.ToSlash(path))
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 2 || paths[0] != "tree" || paths[1] != "tree/a.txt" {
		t.Fatalf("%q", paths)
	}
}

func TestNewDirent(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "a.txt")
	mustWrite(t, file, "a")
	de, err := NewDirent(file)
	if err != nil || !de.IsRegular() || de.Name() != "a.txt" {
		t.Fatalf("%+v %v", de, err)
	}
	dir, err := NewDirent(root)
	if err != nil || !dir.IsDir() {
		t.Fatal(err)
	}
}

func TestWalkPostChildrenRoot(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "child", "x.txt"), "x")
	var post []string
	err := Walk(root, &Options{
		Callback: func(string, *Dirent) error { return nil },
		PostChildrenCallback: func(path string, _ *Dirent) error {
			rel, _ := filepath.Rel(root, path)
			post = append(post, filepath.ToSlash(rel))
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(post, ",") != "child,." {
		t.Fatalf("%q", post)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
