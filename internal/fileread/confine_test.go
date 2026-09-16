package fileread

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfineRejectsSymlinkRoot(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "out")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "x"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "root")
	if err := os.Symlink(outside, root); err != nil {
		t.Skip(err)
	}
	if _, err := Confine(root, filepath.Join(root, "x")); err == nil {
		t.Fatal("symlink root must be rejected")
	}
}

func TestConfineRejectsIntermediateLink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "x"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "hop")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip(err)
	}
	if _, err := Confine(root, filepath.Join(link, "x")); err == nil {
		t.Fatal("intermediate")
	}
}
