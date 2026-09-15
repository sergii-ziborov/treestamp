package compat_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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

func write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeNode(root, path string, entry fs.DirEntry) node {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		relative = path
	}
	if relative == "." {
		relative = ""
	}
	return node{Relative: filepath.ToSlash(relative), Kind: entryKind(entry)}
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
