package pathx

import (
	"os"
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
	extended := `\\?\` + root
	if !UnderRoot(extended, child) || !UnderRoot(root, `\\?\`+child) {
		t.Fatal("extended-length prefix must not change confinement")
	}
}

func TestNativeStripsExtendedPrefix(t *testing.T) {
	if Native(`\\?\C:\temp\root`) != `C:\temp\root` {
		t.Fatal(Native(`\\?\C:\temp\root`))
	}
	if Native(`\\?\UNC\server\share\dir`) != `\\server\share\dir` {
		t.Fatal(Native(`\\?\UNC\server\share\dir`))
	}
	if Native(`/tmp/root`) != `/tmp/root` {
		t.Fatal("unix")
	}
}

func TestJoinLinkReadsRelativeTarget(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "escape")
	target, relErr := filepath.Rel(root, outside)
	if relErr != nil {
		t.Fatal(relErr)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	joined, err := JoinLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if UnderRoot(root, joined) {
		t.Fatalf("joined still under root: %s", joined)
	}
	if !EscapesRoot(root, link) {
		t.Fatal("escape link must leave the root")
	}
}

func TestEscapesRootKeepsDanglingInTree(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "dangling")
	if err := os.Symlink(filepath.Join(root, "missing"), link); err != nil {
		t.Skip(err)
	}
	if EscapesRoot(root, link) {
		t.Fatal("dangling in-tree target is not an escape")
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
