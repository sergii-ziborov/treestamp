package walkfs

import (
	"errors"
	"io"
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"
)

type walkOperation func(fs.FS, string, fs.WalkDirFunc) error
type walkDecision func(string, fs.DirEntry, error) error

func TestWalkMatchesStandard(t *testing.T) {
	assertWalkParity(t, mapTree(), "root", nil)
}

func TestWalkControlParity(t *testing.T) {
	tests := map[string]walkDecision{
		"skip directory": func(name string, _ fs.DirEntry, _ error) error {
			if name == "root/skip" {
				return fs.SkipDir
			}
			return nil
		},
		"skip file parent": func(name string, _ fs.DirEntry, _ error) error {
			if name == "root/dir/a.txt" {
				return fs.SkipDir
			}
			return nil
		},
		"skip all": func(name string, _ fs.DirEntry, _ error) error {
			if name == "root/dir" {
				return fs.SkipAll
			}
			return nil
		},
	}
	for name, decide := range tests {
		t.Run(name, func(t *testing.T) {
			assertWalkParity(t, mapTree(), "root", decide)
		})
	}
}

func TestWalkErrorParity(t *testing.T) {
	readErr := errors.New("partial directory read")
	fsys := partialReadFS{FS: mapTree(), failed: "root/dir", err: readErr}
	t.Run("continue partial", func(t *testing.T) {
		assertWalkParity(t, fsys, "root", nil)
	})
	t.Run("skip partial", func(t *testing.T) {
		assertWalkParity(t, fsys, "root", func(_ string, _ fs.DirEntry, err error) error {
			if err != nil {
				return fs.SkipDir
			}
			return nil
		})
	})
	t.Run("missing root", func(t *testing.T) {
		assertWalkParity(t, fstest.MapFS{}, "missing", nil)
	})
}

func TestPullWalker(t *testing.T) {
	walker, err := New(mapTree(), "root")
	if err != nil {
		t.Fatal(err)
	}
	if walker.Root() != "root" || walker.FS() == nil {
		t.Fatal("root or filesystem accessor mismatch")
	}
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
		assertEntryAccessors(t, entry)
		if entry.Path() == "root/skip" {
			walker.SkipCurrentDir()
		}
	}
	want := []string{"root", "root/a.txt", "root/dir", "root/dir/a.txt", "root/dir/b.txt", "root/skip", "root/z.txt"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
	if err := walker.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := walker.Next(); err != io.EOF {
		t.Fatalf("closed Next error = %v", err)
	}
}

func TestPullReadErrorCanSkipPartial(t *testing.T) {
	fsys := partialReadFS{FS: mapTree(), failed: "root/dir", err: errors.New("read failure")}
	walker, err := New(fsys, "root")
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
		if entry != nil {
			paths = append(paths, entry.Path())
		}
		if nextErr != nil {
			if entry.Path() != "root/dir" {
				t.Fatalf("error entry = %s", entry.Path())
			}
			walker.SkipCurrentDir()
		}
	}
	if contains(paths, "root/dir/a.txt") || !contains(paths, "root/z.txt") {
		t.Fatalf("unexpected paths after read-error skip: %#v", paths)
	}
}

func TestInvalidFSInputs(t *testing.T) {
	if _, err := New(nil, "."); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("nil filesystem error = %v", err)
	}
	if _, err := New(mapTree(), "/root"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("invalid root error = %v", err)
	}
	if err := Walk(mapTree(), "root", nil); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("nil callback error = %v", err)
	}
}

func assertEntryAccessors(t *testing.T, entry *Entry) {
	t.Helper()
	if entry.DirEntry() == nil || entry.FileName() != entry.DirEntry().Name() {
		t.Fatalf("bad directory entry for %s", entry.Path())
	}
	if entry.Path() == "root" {
		if entry.Depth() != 0 || entry.RelativePath() != "" || !entry.IsDir() {
			t.Fatalf("bad root entry: %#v", entry)
		}
		return
	}
	if entry.Depth() < 1 || entry.RelativePath() == "" {
		t.Fatalf("bad child entry: %#v", entry)
	}
	_, _ = entry.Info()
	_ = entry.Type()
	_ = entry.IsFile()
	_ = entry.IsSymlink()
}

func assertWalkParity(t *testing.T, fsys fs.FS, root string, decide walkDecision) {
	t.Helper()
	want, wantErr := collectWalk(fsys, root, fs.WalkDir, decide)
	got, gotErr := collectWalk(fsys, root, Walk, decide)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events differ\nwant: %#v\n got: %#v", want, got)
	}
	if errorText(gotErr) != errorText(wantErr) {
		t.Fatalf("errors differ: want %v, got %v", wantErr, gotErr)
	}
}

func collectWalk(fsys fs.FS, root string, operation walkOperation, decide walkDecision) ([]string, error) {
	var events []string
	err := operation(fsys, root, func(name string, entry fs.DirEntry, err error) error {
		events = append(events, eventLabel(name, entry, err))
		if decide != nil {
			return decide(name, entry, err)
		}
		return nil
	})
	return events, err
}

func eventLabel(name string, entry fs.DirEntry, err error) string {
	kind := "nil"
	if entry != nil {
		kind = entry.Type().String()
		if entry.IsDir() {
			kind = "dir"
		}
	}
	return name + "|" + kind + "|" + errorText(err)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func mapTree() fstest.MapFS {
	return fstest.MapFS{
		"root/a.txt":         {Data: []byte("a")},
		"root/dir/a.txt":     {Data: []byte("a")},
		"root/dir/b.txt":     {Data: []byte("b")},
		"root/skip/file.txt": {Data: []byte("skip")},
		"root/z.txt":         {Data: []byte("z")},
	}
}

type partialReadFS struct {
	fs.FS
	failed string
	err    error
}

func (p partialReadFS) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(p.FS, name)
	if err != nil || name != p.failed {
		return entries, err
	}
	if len(entries) > 1 {
		entries = entries[:1]
	}
	return entries, p.err
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
