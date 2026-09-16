package compat_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
)

type node struct {
	Relative string
	Kind     string
}

func makeTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, "a.txt"), "a")
	write(t, filepath.Join(root, "z.txt"), "z")
	write(t, filepath.Join(root, "keep", "b.txt"), "b")
	write(t, filepath.Join(root, "skip", "hidden.txt"), "hidden")
	write(t, filepath.Join(root, "deep", "one", "c.go"), "package c")
	return root
}

func write(tb testing.TB, path, contents string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		tb.Fatal(err)
	}
}

func makeSelectiveCorpus(tb testing.TB, groups int) string {
	tb.Helper()
	root := tb.TempDir()
	for i := 0; i < groups; i++ {
		group := filepath.Join(root, fmt.Sprintf("g%02d", i))
		write(tb, filepath.Join(group, "chosen-target", "file.txt"), "c")
		write(tb, filepath.Join(group, "ignored-target", "file.txt"), "i")
		mustSymlink(tb, filepath.Join(group, "chosen-target"), filepath.Join(group, "chosen"))
		mustSymlink(tb, filepath.Join(group, "ignored-target"), filepath.Join(group, "ignored"))
	}
	return root
}

func makeWideDir(tb testing.TB, files int) string {
	tb.Helper()
	root := tb.TempDir()
	for i := 0; i < files; i++ {
		write(tb, filepath.Join(root, fmt.Sprintf("f%04d.txt", i)), "x")
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		tb.Fatal(err)
	}
	return root
}

func makeStatCorpus(tb testing.TB, files, links int) string {
	tb.Helper()
	root := tb.TempDir()
	for i := 0; i < files; i++ {
		write(tb, filepath.Join(root, "files", fmt.Sprintf("f%03d.txt", i)), "x")
	}
	if links > 0 {
		if err := os.MkdirAll(filepath.Join(root, "links"), 0o755); err != nil {
			tb.Fatal(err)
		}
	}
	for i := 0; i < links; i++ {
		target := filepath.Join(root, "files", fmt.Sprintf("f%03d.txt", i%files))
		mustSymlink(tb, target, filepath.Join(root, "links", fmt.Sprintf("l%03d", i)))
	}
	return root
}

func countWalk(walk func(string, fs.WalkDirFunc) error, root string, visit fs.WalkDirFunc) (int, error) {
	var n atomic.Int64
	err := walk(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		n.Add(1)
		if visit != nil {
			return visit(path, entry, nil)
		}
		return nil
	})
	return int(n.Load()), err
}

func benchRepeat(b *testing.B, run func() (int, error)) {
	b.Helper()
	b.ReportAllocs()
	seen := 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, err := run()
		if err != nil {
			b.Fatal(err)
		}
		seen = n
	}
	if seen == 0 {
		b.Fatal("empty walk")
	}
}

func makeNode(root, path string, entry fs.DirEntry) node {
	return node{Relative: relPath(root, path), Kind: entryKind(entry)}
}

func relPath(root, path string) string {
	if value, ok := relUnder(root, path); ok {
		return value
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		if value, ok := relUnder(resolved, path); ok {
			return value
		}
	}
	value, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return slashRel(value)
}

func relUnder(root, path string) (string, bool) {
	value, err := filepath.Rel(root, path)
	if err != nil || escapesRoot(value) {
		return "", false
	}
	return slashRel(value), true
}

func escapesRoot(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func slashRel(value string) string {
	if value == "." {
		return ""
	}
	return filepath.ToSlash(value)
}

func entryKind(entry fs.DirEntry) string {
	switch {
	case entry.Type()&os.ModeSymlink != 0:
		return "symlink"
	case entry.IsDir():
		return "dir"
	case entry.Type().IsRegular():
		return "file"
	default:
		return "other"
	}
}

func sortedNodes(nodes []node) []node {
	out := append([]node(nil), nodes...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Relative != out[j].Relative {
			return out[i].Relative < out[j].Relative
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func assertNodeSet(t *testing.T, want []node, named map[string][]node) {
	t.Helper()
	expected := sortedNodes(want)
	for name, got := range named {
		actual := sortedNodes(got)
		if !equalNodes(expected, actual) {
			t.Errorf("%s nodes differ\nwant: %#v\n got: %#v", name, expected, actual)
		}
	}
}

func equalNodes(left, right []node) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func assertPaths(t *testing.T, want []string, named map[string][]string) {
	t.Helper()
	expected := append([]string(nil), want...)
	sort.Strings(expected)
	for name, got := range named {
		actual := append([]string(nil), got...)
		sort.Strings(actual)
		if !equalStrings(expected, actual) {
			t.Errorf("%s paths differ\nwant: %#v\n got: %#v", name, expected, actual)
		}
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func TestRelPathKeepsWalkName(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "chosen", "file.txt")
	if got := relPath(root, link); got != "chosen/file.txt" {
		t.Fatalf("relPath = %q", got)
	}
	if got := relPath(root, filepath.Join(root, "..", "002", "outside.txt")); strings.Contains(got, "escape") {
		t.Fatalf("escaped path should not invent a link name: %q", got)
	}
}
