package compat_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/charlievieth/fastwalk"
	"github.com/karrick/godirwalk"
	"github.com/sergii-ziborov/treestamp"
)

func TestRawWalkNodeSetParity(t *testing.T) {
	root := makeTree(t)
	std, err := collectStd(root, "")
	if err != nil {
		t.Fatal(err)
	}
	tree, treeErr := collectTreestamp(root, "", false)
	fast, fastErr := collectFastwalk(root, "", false)
	godir, godirErr := collectGodirwalk(root, "", false, runtime.GOOS != "windows")
	requireWalkErrors(t, treeErr, fastErr, godirErr)
	assertNodeSet(t, std, map[string][]node{
		"treestamp": tree,
		"fastwalk":  fast,
		"godirwalk": godir,
	})
}

func TestSkipDirectoryParity(t *testing.T) {
	root := makeTree(t)
	std, err := collectStd(root, "skip")
	if err != nil {
		t.Fatal(err)
	}
	tree, treeErr := collectTreestamp(root, "skip", false)
	fast, fastErr := collectFastwalk(root, "skip", false)
	godir, godirErr := collectGodirwalk(root, "skip", false, runtime.GOOS != "windows")
	requireWalkErrors(t, treeErr, fastErr, godirErr)
	assertNodeSet(t, std, map[string][]node{
		"treestamp": tree,
		"fastwalk":  fast,
		"godirwalk": godir,
	})
	for _, item := range std {
		if item.Relative == "skip/hidden.txt" {
			t.Fatal("skip directory was traversed")
		}
	}
}

func TestSortedDepthFirstParity(t *testing.T) {
	root := makeTree(t)
	var std []string
	if err := filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err == nil {
			std = append(std, relative(root, path))
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	tree, err := sortedTreestamp(root)
	if err != nil {
		t.Fatal(err)
	}
	godir, err := sortedGodirwalk(root)
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(std, tree) {
		t.Errorf("treestamp sequence differs\nwant: %#v\n got: %#v", std, tree)
	}
	if !equalStrings(std, godir) {
		t.Errorf("godirwalk sequence differs\nwant: %#v\n got: %#v", std, godir)
	}
}

func TestCallbackStopParity(t *testing.T) {
	root := makeTree(t)
	stop := errors.New("stop walk")
	callback := func(string, fs.DirEntry, error) error { return stop }
	if !errors.Is(filepath.WalkDir(root, callback), stop) {
		t.Fatal("filepath did not return callback error")
	}
	if !errors.Is(treestamp.WalkWithConfig(root, treestamp.Config{NumWorkers: 2}, callback), stop) {
		t.Fatal("treestamp parallel walk lost callback error")
	}
	if !errors.Is(fastwalk.Walk(&fastwalk.Config{NumWorkers: 2}, root, callback), stop) {
		t.Fatal("fastwalk did not return callback error")
	}
	err := godirwalk.Walk(root, &godirwalk.Options{
		Callback: func(string, *godirwalk.Dirent) error { return stop },
	})
	if !errors.Is(err, stop) {
		t.Fatal("godirwalk did not return callback error")
	}
}

func TestGodirwalkUnsortedStatus(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "file.txt"), "x")
	nodes, err := collectGodirwalk(root, "", false, true)
	if runtime.GOOS == "windows" {
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected documented godirwalk Windows EOF, got %v", err)
		}
		t.Logf("documented difference: godirwalk v1.17.0 Unsorted returned EOF after %d nodes", len(nodes))
		return
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestDirectorySymlinkFollowParity(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "target", "file.txt"), "x")
	link := filepath.Join(root, "alias")
	if err := os.Symlink(filepath.Join(root, "target"), link); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	tree, treeErr := collectTreestamp(root, "", true)
	fast, fastErr := collectFastwalk(root, "", true)
	godir, godirErr := collectGodirwalk(root, "", true, false)
	requireWalkErrors(t, treeErr, fastErr, godirErr)
	assertNodeSet(t, tree, map[string][]node{"fastwalk": fast, "godirwalk": godir})
}

func collectStd(root, skip string) ([]node, error) {
	var out []node
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out = append(out, makeNode(root, path, entry))
		if skip != "" && relative(root, path) == skip && entry.IsDir() {
			return fs.SkipDir
		}
		return nil
	})
	return out, err
}

func collectTreestamp(root, skip string, follow bool) ([]node, error) {
	var mu sync.Mutex
	var out []node
	err := treestamp.WalkWithConfig(root, treestamp.Config{Follow: follow, NumWorkers: 2}, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		mu.Lock()
		out = append(out, makeNode(root, path, entry))
		mu.Unlock()
		if skip != "" && relative(root, path) == skip && entry.IsDir() {
			return fs.SkipDir
		}
		return nil
	})
	return out, err
}

func collectFastwalk(root, skip string, follow bool) ([]node, error) {
	var mu sync.Mutex
	var out []node
	err := fastwalk.Walk(&fastwalk.Config{Follow: follow, NumWorkers: 2}, root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		mu.Lock()
		out = append(out, makeNode(root, path, entry))
		mu.Unlock()
		if skip != "" && relative(root, path) == skip && entry.IsDir() {
			return fastwalk.SkipDir
		}
		return nil
	})
	return out, err
}

func collectGodirwalk(root, skip string, follow, unsorted bool) ([]node, error) {
	var out []node
	err := godirwalk.Walk(root, &godirwalk.Options{
		FollowSymbolicLinks: follow,
		Unsorted:            unsorted,
		Callback: func(path string, entry *godirwalk.Dirent) error {
			out = append(out, node{Relative: relative(root, path), Kind: godirKind(entry)})
			if skip != "" && relative(root, path) == skip && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		},
	})
	return out, err
}

func sortedTreestamp(root string) ([]string, error) {
	var out []string
	err := treestamp.WalkWithConfig(root, treestamp.Config{Sort: true, NumWorkers: 2}, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out = append(out, relative(root, path))
		return nil
	})
	return out, err
}

func sortedGodirwalk(root string) ([]string, error) {
	var out []string
	err := godirwalk.Walk(root, &godirwalk.Options{
		Callback: func(path string, _ *godirwalk.Dirent) error {
			out = append(out, relative(root, path))
			return nil
		},
	})
	return out, err
}

func relative(root, path string) string {
	value, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	if value == "." {
		return ""
	}
	return filepath.ToSlash(value)
}

func godirKind(entry *godirwalk.Dirent) string {
	switch {
	case entry.IsSymlink():
		return "symlink"
	case entry.IsDir():
		return "dir"
	case entry.IsRegular():
		return "file"
	default:
		return "other"
	}
}

func requireWalkErrors(t *testing.T, tree, fast, godir error) {
	t.Helper()
	for name, err := range map[string]error{
		"treestamp": tree,
		"fastwalk":  fast,
		"godirwalk": godir,
	} {
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestFilesFirstWalkOrder(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a.txt"), "a")
	write(t, filepath.Join(root, "sub", "b.txt"), "b")
	var filesBeforeDir bool
	var sawFile, sawDir bool
	err := treestamp.WalkDirs(root, treestamp.DirWalkOptions{
		ContentsFirst: true,
		Callback: func(path string, d fs.DirEntry, err error) error {
			if err != nil || d == nil {
				return err
			}
			rel := relative(root, path)
			if rel == "a.txt" {
				sawFile = true
				if !sawDir {
					filesBeforeDir = true
				}
			}
			if rel == "sub" {
				sawDir = true
			}
			return nil
		},
	})
	if err != nil || !sawFile || !sawDir || !filesBeforeDir {
		t.Fatalf("order file=%v dir=%v first=%v err=%v", sawFile, sawDir, filesBeforeDir, err)
	}
}

func BenchmarkRawWalkTreestampSerial(b *testing.B) {
	root := makeWideDir(b, 400)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := treestamp.Walk(root, func(string, fs.DirEntry, error) error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRawWalkTreestampParallel(b *testing.B) {
	root := makeWideDir(b, 400)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := treestamp.WalkUnsorted(root, func(string, fs.DirEntry, error) error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRawWalkFastwalk(b *testing.B) {
	root := makeWideDir(b, 400)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := fastwalk.Walk(nil, root, func(string, fs.DirEntry, error) error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRawWalkGodirwalk(b *testing.B) {
	root := makeWideDir(b, 400)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := godirwalk.Walk(root, &godirwalk.Options{
			Unsorted: runtime.GOOS != "windows",
			Callback: func(string, *godirwalk.Dirent) error { return nil },
		}); err != nil && !errors.Is(err, io.EOF) {
			b.Fatal(err)
		}
	}
}
