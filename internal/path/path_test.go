package pathx

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestIsSameOrDescendant(t *testing.T) {
	if !IsSameOrDescendant("src", "src") {
		t.Fatal("src")
	}
	if !IsSameOrDescendant("src/nested/a.rs", "src") {
		t.Fatal("nested")
	}
	if IsSameOrDescendant("src2", "src") {
		t.Fatal("src2 must not match src")
	}
	if IsSameOrDescendant("src", "src/nested") {
		t.Fatal("ancestor must not match longer prefix")
	}
}

func TestSlashAndUnderRoot(t *testing.T) {
	if Slash(`a\b`) == "" {
		t.Fatal("slash")
	}
	root := t.TempDir()
	if !UnderRoot(root, root) {
		t.Fatal("root is under itself")
	}
	child := filepath.Join(root, "child")
	if !UnderRoot(root, child) {
		t.Fatal("child")
	}
	if UnderRoot(root, filepath.Join(root, "..", "outside")) {
		t.Fatal("parent must not be under root")
	}
}

func TestCollapsePathPrefixes(t *testing.T) {
	got := CollapsePathPrefixes([]string{"src/nested", "src", "src", "src2"})
	want := []string{"src", "src2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if !PathCoveredByPrefixes("src/a.rs", got) {
		t.Fatal("src/a.rs")
	}
	if PathCoveredByPrefixes("src3/c.rs", got) {
		t.Fatal("src3")
	}
}

func TestSafeRelative(t *testing.T) {
	if got, ok := SafeRelative("src/a.go"); !ok || got != "src/a.go" {
		t.Fatal(got, ok)
	}
	if got, ok := SafeRelative("."); !ok || got != "." {
		t.Fatal(got, ok)
	}
	if _, ok := SafeRelative(".."); ok {
		t.Fatal("parent")
	}
	if _, ok := SafeRelative("src/../secret"); ok {
		t.Fatal("escape")
	}
	if _, ok := SafeRelative("/abs"); ok {
		t.Fatal("abs")
	}
}
