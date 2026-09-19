package walkcfg

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestHardlinksAreOneObject(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	b := filepath.Join(root, "b.txt")
	if err := os.WriteFile(a, []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(a, b); err != nil {
		t.Skip(err)
	}
	ai, err := os.Lstat(a)
	if err != nil {
		t.Fatal(err)
	}
	bi, err := os.Lstat(b)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(ai, bi) {
		t.Fatal("test tree is not one object")
	}
	var names []string
	err = Serial(root, Config{}, IgnoreDuplicateFiles(func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		names = append(names, d.Name())
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 {
		t.Fatalf("hardlinks delivered %v", names)
	}
}

func TestSameBytesStayDistinct(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var n int
	err := Serial(root, Config{}, IgnoreDuplicateFiles(func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		n++
		return nil
	}))
	if err != nil || n != 2 {
		t.Fatalf("files=%d err=%v", n, err)
	}
}

func TestIgnoreDuplicateDirsWalksChildrenOnce(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dir", "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("dir", filepath.Join(root, "alias")); err != nil {
		t.Skip(err)
	}
	var files []string
	var aliases int
	err := Serial(root, Config{}, IgnoreDuplicateDirs(func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			aliases++
		}
		if d.Type().IsRegular() {
			files = append(files, d.Name())
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if aliases == 0 {
		t.Fatal("alias was not shown to the callback")
	}
	if len(files) != 1 {
		t.Fatalf("children walked %v (want one x.txt)", files)
	}
}

func TestEntryFilterDoesNotInventIdentity(t *testing.T) {
	filter := NewEntryFilter()
	seen, ok := filter.Observe(filepath.Join(t.TempDir(), "missing"), nil)
	if seen || ok {
		t.Fatalf("seen=%v ok=%v", seen, ok)
	}
}

func TestAliasIsDirectoryTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink("dir", alias); err != nil {
		t.Skip(err)
	}
	info, err := os.Lstat(alias)
	if err != nil {
		t.Fatal(err)
	}
	if !isDirOrLinkToDir(alias, fs.FileInfoToDirEntry(info)) {
		t.Fatal("expected directory-target request")
	}
}
