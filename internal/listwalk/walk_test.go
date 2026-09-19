package listwalk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

var testSkipThis = errors.New("skip this directory entry")

func TestWalkKeepsSavedEntries(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	var saved []fs.DirEntry
	if err := Walk(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		saved = append(saved, d)
		return nil
	}, Config{}); err != nil {
		t.Fatal(err)
	}
	if len(saved) < 2 {
		t.Fatalf("saved %d", len(saved))
	}
	for _, d := range saved {
		if d.Name() == "" {
			t.Fatal("cleared entry")
		}
	}
}

func TestWalkPostsRootAfterChildren(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "child", "x.txt"), "x")
	var events []string
	err := Walk(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		events = append(events, "pre:"+eventName(root, path))
		return nil
	}, Config{After: func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		events = append(events, "post:"+eventName(root, path))
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !equalEvents(events, []string{"pre:.", "pre:child", "pre:child/x.txt", "post:child", "post:."}) {
		t.Fatalf("%q", events)
	}
}

func TestWalkPostsEmptyRoot(t *testing.T) {
	root := t.TempDir()
	var events []string
	err := Walk(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		events = append(events, "pre:"+eventName(root, path))
		return nil
	}, Config{After: func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		events = append(events, "post:"+eventName(root, path))
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !equalEvents(events, []string{"pre:.", "post:."}) {
		t.Fatalf("%q", events)
	}
}

func TestWalkSkipRootHasNoPost(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "child"), "x")
	var events []string
	err := Walk(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		events = append(events, "pre:"+eventName(root, path))
		if eventName(root, path) == "." {
			return fs.SkipDir
		}
		return nil
	}, Config{After: func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		events = append(events, "post:"+eventName(root, path))
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !equalEvents(events, []string{"pre:."}) {
		t.Fatalf("%q", events)
	}
}

func TestWalkSortsChildren(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "b.txt"), "b")
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	mustWrite(t, filepath.Join(root, "m", "z.txt"), "z")
	mustWrite(t, filepath.Join(root, "m", "a.txt"), "a")
	var names []string
	if err := Walk(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		if name := eventName(root, path); name != "." {
			names = append(names, name)
		}
		return nil
	}, Config{Sort: true}); err != nil {
		t.Fatal(err)
	}
	want := []string{"a.txt", "b.txt", "m", "m/a.txt", "m/z.txt"}
	if !equalEvents(names, want) {
		t.Fatalf("%q != %q", names, want)
	}
}

func TestWalkSkipThisKeepsSiblings(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	mustWrite(t, filepath.Join(root, "b.txt"), "b")
	mustWrite(t, filepath.Join(root, "skip", "hidden.txt"), "h")
	var names []string
	err := Walk(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		name := eventName(root, path)
		if name == "a.txt" || name == "skip" {
			return testSkipThis
		}
		if name != "." {
			names = append(names, name)
		}
		return nil
	}, Config{Sort: true, SkipThis: testSkipThis})
	if err != nil {
		t.Fatal(err)
	}
	if !equalEvents(names, []string{"b.txt"}) {
		t.Fatalf("%q", names)
	}
}

func TestWalkKeepsRelativeRoot(t *testing.T) {
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
	if err := Walk("tree", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(path))
		return nil
	}, Config{Sort: true}); err != nil {
		t.Fatal(err)
	}
	if len(paths) < 2 || paths[0] != "tree" || paths[1] != "tree/a.txt" {
		t.Fatalf("%q", paths)
	}
}

func TestWalkRequireDirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "only.txt")
	mustWrite(t, file, "x")
	err := Walk(file, func(string, fs.DirEntry, error) error { return nil }, Config{RequireDirectory: true})
	if err == nil {
		t.Fatal("expected non-directory")
	}
}

func TestWalkRegularTypeWithoutInfo(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	var files int
	if err := Walk(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		if !d.Type().IsRegular() {
			t.Fatalf("type=%v", d.Type())
		}
		files++
		return nil
	}, Config{Sort: true}); err != nil {
		t.Fatal(err)
	}
	if files != 1 {
		t.Fatalf("files=%d", files)
	}
}

func BenchmarkWalkLexical(b *testing.B) {
	root := b.TempDir()
	for i := 0; i < 64; i++ {
		mustWrite(b, filepath.Join(root, "f"+strconv.Itoa(i)+".txt"), "x")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Walk(root, func(string, fs.DirEntry, error) error { return nil }, Config{Sort: true}); err != nil {
			b.Fatal(err)
		}
	}
}

func eventName(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func equalEvents(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func mustWrite(t testing.TB, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
