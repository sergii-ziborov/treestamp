package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHiddenNameAndIdentity(t *testing.T) {
	if !HiddenName(".git") || HiddenName("keep.go") {
		t.Fatal("hidden name")
	}
	if !Hidden(".hidden", nil) {
		t.Fatal("hidden path")
	}
	native := true
	if !Hidden("keep.go", &native) {
		t.Fatal("native")
	}
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := PathIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	match, err := PathMatchesIdentity(path, id)
	if err != nil || !match {
		t.Fatalf("match %v %v", match, err)
	}
	if !id.Equal(id) {
		t.Fatal("equal")
	}
	_, _ = StdoutIdentity()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = HiddenFromInfo(path, info)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := FileIdentityFromFile(f); err != nil {
		t.Fatal(err)
	}
	if _, err := DirectoryInfo(root); err != nil {
		t.Fatal(err)
	}
}
