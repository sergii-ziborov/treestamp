package compat_test

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/karrick/godirwalk"
	"github.com/sergii-ziborov/treestamp"
)

func TestArbitraryFSWalkParity(t *testing.T) {
	fsys := virtualTree()
	want, wantErr := collectFSPaths(fsys, "repo", fs.WalkDir)
	got, gotErr := collectFSPaths(fsys, "repo", treestamp.WalkFS)
	if wantErr != nil || gotErr != nil {
		t.Fatalf("walk errors: standard=%v treestamp=%v", wantErr, gotErr)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths differ\nwant: %#v\n got: %#v", want, got)
	}
}

func TestArbitraryFSControlParity(t *testing.T) {
	fsys := virtualTree()
	decide := func(name string, _ fs.DirEntry, _ error) error {
		if name == "repo/skip" {
			return fs.SkipDir
		}
		return nil
	}
	want, wantErr := collectControlledFS(fsys, "repo", fs.WalkDir, decide)
	got, gotErr := collectControlledFS(fsys, "repo", treestamp.WalkFS, decide)
	if wantErr != nil || gotErr != nil {
		t.Fatalf("walk errors: standard=%v treestamp=%v", wantErr, gotErr)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("controlled paths differ\nwant: %#v\n got: %#v", want, got)
	}
}

func TestArbitraryFSPullWalker(t *testing.T) {
	sub, err := fs.Sub(virtualTree(), "repo")
	if err != nil {
		t.Fatal(err)
	}
	walker, err := treestamp.NewFSWalker(sub, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	var paths []string
	for {
		entry, nextErr := walker.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			t.Fatal(nextErr)
		}
		paths = append(paths, entry.Path())
		if entry.Path() == "skip" {
			walker.SkipCurrentDir()
		}
	}
	want := []string{".", "a.txt", "dir", "dir/b.txt", "link", "skip"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestArbitraryFSDirFSSymlinkParity(t *testing.T) {
	base := t.TempDir()
	write(t, filepath.Join(base, "target", "file.txt"), "x")
	mustSymlink(t, "target", filepath.Join(base, "alias"))
	fsys := os.DirFS(base)
	paths := assertFSRootParity(t, fsys, ".")
	if hasFSEvent(paths, "alias/file.txt") {
		t.Fatal("WalkFS followed a child symlink")
	}

	mustSymlink(t, "target", filepath.Join(base, "root-link"))
	paths = assertFSRootParity(t, fsys, "root-link")
	if !hasFSEvent(paths, "root-link/file.txt") {
		t.Fatal("WalkFS did not follow the root symlink")
	}
}

type fsWalkOperation func(fs.FS, string, fs.WalkDirFunc) error

func collectFSPaths(fsys fs.FS, root string, operation fsWalkOperation) ([]string, error) {
	return collectControlledFS(fsys, root, operation, nil)
}

func collectControlledFS(fsys fs.FS, root string, operation fsWalkOperation, decide walkDecisionFS) ([]string, error) {
	var paths []string
	err := operation(fsys, root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, name+":"+entry.Type().String())
		if decide != nil {
			return decide(name, entry, nil)
		}
		return nil
	})
	return paths, err
}

func assertFSRootParity(t *testing.T, fsys fs.FS, root string) []string {
	t.Helper()
	want, wantErr := collectFSPaths(fsys, root, fs.WalkDir)
	got, gotErr := collectFSPaths(fsys, root, treestamp.WalkFS)
	if wantErr != nil || gotErr != nil {
		t.Fatalf("walk errors: standard=%v treestamp=%v", wantErr, gotErr)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths differ\nwant: %#v\n got: %#v", want, got)
	}
	return got
}

func hasFSEvent(events []string, name string) bool {
	for _, event := range events {
		if strings.HasPrefix(event, name+":") {
			return true
		}
	}
	return false
}

type walkDecisionFS func(string, fs.DirEntry, error) error

func virtualTree() fstest.MapFS {
	return fstest.MapFS{
		"repo/a.txt":         {Data: []byte("a")},
		"repo/dir/b.txt":     {Data: []byte("b")},
		"repo/link":          {Mode: fs.ModeSymlink},
		"repo/skip/file.txt": {Data: []byte("skip")},
	}
}

func wideMapFS() fstest.MapFS {
	fsys := fstest.MapFS{}
	for dir := 0; dir < 40; dir++ {
		for file := 0; file < 50; file++ {
			name := fmt.Sprintf("d%02d/f%02d.txt", dir, file)
			fsys[name] = &fstest.MapFile{Data: []byte("x")}
		}
	}
	return fsys
}

func BenchmarkArbitraryFSWalkDir(b *testing.B) {
	benchArbitraryFS(b, fs.WalkDir)
}

func BenchmarkArbitraryFSWalkFS(b *testing.B) {
	benchArbitraryFS(b, treestamp.WalkFS)
}

func benchArbitraryFS(b *testing.B, walk fsWalkOperation) {
	fsys := wideMapFS()
	benchRepeat(b, func() (int, error) {
		n := 0
		err := walk(fsys, ".", func(_ string, _ fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			n++
			return nil
		})
		return n, err
	})
}

func TestReadDirentsScratchParity(t *testing.T) {
	root := makeTree(t)
	scratch := make([]byte, sharedScratchSize())
	want, err := godirwalk.ReadDirents(root, scratch)
	if err != nil {
		t.Fatal(err)
	}
	got, err := treestamp.ReadDirentsScratch(root, scratch)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(treestampDirentKeys(got), godirwalkDirentKeys(want)) {
		t.Fatalf("dirents differ\nwant: %#v\n got: %#v", godirwalkDirentKeys(want), treestampDirentKeys(got))
	}
}

func TestReadDirnamesParity(t *testing.T) {
	root := makeTree(t)
	scratch := make([]byte, sharedScratchSize())
	want, err := godirwalk.ReadDirnames(root, scratch)
	if err != nil {
		t.Fatal(err)
	}
	got, err := treestamp.ReadDirnames(root, scratch)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(want)
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("names differ\nwant: %#v\n got: %#v", want, got)
	}
}

func TestDirScannerParity(t *testing.T) {
	root := makeTree(t)
	scratch := make([]byte, sharedScratchSize())
	want := collectGodirwalkScanner(t, root, scratch)
	got := collectTreestampScanner(t, root, scratch)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scanner entries differ\nwant: %#v\n got: %#v", want, got)
	}
}

func TestScratchBufferReuse(t *testing.T) {
	root := makeTree(t)
	scratch := treestamp.NewScratchBuffer()
	dirs := []string{root, filepath.Join(root, "keep"), filepath.Join(root, "skip"), filepath.Join(root, "deep")}
	for _, dir := range dirs {
		want, err := godirwalk.ReadDirnames(dir, scratch)
		if err != nil {
			t.Fatal(err)
		}
		got, err := treestamp.ReadDirnames(dir, scratch)
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(want)
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s names differ\nwant: %#v\n got: %#v", dir, want, got)
		}
	}
}

func TestDirScannerEmptyAndMissing(t *testing.T) {
	empty := t.TempDir()
	scanner, err := treestamp.NewDirScanner(empty)
	if err != nil {
		t.Fatal(err)
	}
	if scanner.Scan() {
		t.Fatal("empty directory yielded an entry")
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := treestamp.NewDirScanner(filepath.Join(empty, "missing")); err == nil {
		t.Fatal("missing directory opened")
	}
}

func BenchmarkGodirwalkReadDirnames(b *testing.B) {
	benchReadDirnames(b, func(dir string, scratch []byte) (int, error) {
		names, err := godirwalk.ReadDirnames(dir, scratch)
		return len(names), err
	})
}

func BenchmarkTreestampReadDirnames(b *testing.B) {
	benchReadDirnames(b, func(dir string, scratch []byte) (int, error) {
		names, err := treestamp.ReadDirnames(dir, scratch)
		return len(names), err
	})
}

func BenchmarkGodirwalkReadDirents(b *testing.B) {
	benchReadDirents(b, func(dir string, scratch []byte) (int, error) {
		ents, err := godirwalk.ReadDirents(dir, scratch)
		return len(ents), err
	})
}

func BenchmarkTreestampReadDirentsScratch(b *testing.B) {
	benchReadDirents(b, func(dir string, scratch []byte) (int, error) {
		ents, err := treestamp.ReadDirentsScratch(dir, scratch)
		return len(ents), err
	})
}

func BenchmarkGodirwalkScanner(b *testing.B) {
	benchDirScanner(b, func(dir string, scratch []byte) (int, error) {
		scanner, err := godirwalk.NewScannerWithScratchBuffer(dir, scratch)
		if err != nil {
			return 0, err
		}
		n := 0
		for scanner.Scan() {
			n++
		}
		err = scanner.Err()
		if err == io.EOF {
			err = nil
		}
		return n, err
	})
}

func BenchmarkTreestampDirScanner(b *testing.B) {
	benchDirScanner(b, func(dir string, scratch []byte) (int, error) {
		scanner, err := treestamp.NewDirScannerScratch(dir, scratch)
		if err != nil {
			return 0, err
		}
		n := 0
		for scanner.Scan() {
			n++
		}
		return n, scanner.Err()
	})
}

func benchReadDirents(b *testing.B, read func(string, []byte) (int, error)) {
	root := makeWideDir(b, 4000)
	scratch := make([]byte, sharedScratchSize())
	benchRepeat(b, func() (int, error) { return read(root, scratch) })
}

func benchReadDirnames(b *testing.B, read func(string, []byte) (int, error)) {
	root := makeWideDir(b, 4000)
	scratch := make([]byte, sharedScratchSize())
	benchRepeat(b, func() (int, error) { return read(root, scratch) })
}

func benchDirScanner(b *testing.B, scan func(string, []byte) (int, error)) {
	root := makeWideDir(b, 4000)
	scratch := make([]byte, sharedScratchSize())
	benchRepeat(b, func() (int, error) { return scan(root, scratch) })
}

func collectTreestampScanner(t *testing.T, dir string, scratch []byte) []string {
	t.Helper()
	scanner, err := treestamp.NewDirScannerScratch(dir, scratch)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for scanner.Scan() {
		dent, err := scanner.Dirent()
		if err != nil {
			t.Fatal(err)
		}
		if dent.Name() != scanner.Name() {
			t.Fatalf("name %q vs %q", dent.Name(), scanner.Name())
		}
		out = append(out, dent.Name()+":"+dent.Type().String())
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func collectGodirwalkScanner(t *testing.T, dir string, scratch []byte) []string {
	t.Helper()
	scanner, err := godirwalk.NewScannerWithScratchBuffer(dir, scratch)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for scanner.Scan() {
		dent, err := scanner.Dirent()
		if err != nil {
			t.Fatal(err)
		}
		if dent.Name() != scanner.Name() {
			t.Fatalf("name %q vs %q", dent.Name(), scanner.Name())
		}
		out = append(out, dent.Name()+":"+dent.ModeType().String())
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func treestampDirentKeys(ents []os.DirEntry) []string {
	out := make([]string, len(ents))
	for i, ent := range ents {
		out[i] = ent.Name() + ":" + ent.Type().String()
	}
	sort.Strings(out)
	return out
}

func godirwalkDirentKeys(ents godirwalk.Dirents) []string {
	out := make([]string, len(ents))
	for i, ent := range ents {
		out[i] = ent.Name() + ":" + ent.ModeType().String()
	}
	sort.Strings(out)
	return out
}

func sharedScratchSize() int {
	n := godirwalk.MinimumScratchBufferSize
	if m := treestamp.MinimumScratchBufferSize(); m > n {
		n = m
	}
	if n < 32768 {
		n = 32768
	}
	return n
}
