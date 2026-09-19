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
	if fromInfo, ok := IdentityFromInfo(info); ok && !fromInfo.Equal(id) {
		t.Fatal("IdentityFromInfo must match PathIdentity")
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

func TestIdentityDistinguishesFilesAndHardlinks(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	b := filepath.Join(root, "b.txt")
	if err := os.WriteFile(a, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	ia, err := PathIdentity(a)
	if err != nil {
		t.Fatal(err)
	}
	ib, err := PathIdentity(b)
	if err != nil {
		t.Fatal(err)
	}
	if ia.Equal(ib) {
		t.Fatal("distinct files must not share identity")
	}
	infoA, err := DirectoryInfo(root)
	if err != nil {
		t.Fatal(err)
	}
	if infoA.FileSystem == 0 && infoA.Identity.File == 0 {
		t.Fatal("directory identity")
	}
	link := filepath.Join(root, "link.txt")
	if err := os.Link(a, link); err != nil {
		t.Skip(err)
	}
	il, err := PathIdentity(link)
	if err != nil {
		t.Fatal(err)
	}
	if !ia.Equal(il) {
		t.Fatal("hardlink must share identity")
	}
}

func TestSameVolumeKeepsFileSystemID(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "nested")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	rootInfo, err := DirectoryInfo(root)
	if err != nil {
		t.Fatal(err)
	}
	childInfo, err := DirectoryInfo(child)
	if err != nil {
		t.Fatal(err)
	}
	if rootInfo.FileSystem != childInfo.FileSystem {
		t.Fatal("same volume must share filesystem id")
	}
}

func TestEnumerationInfoDoesNotInventIdentity(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "e.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dents, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var info os.FileInfo
	for _, dent := range dents {
		if dent.Name() != "e.txt" {
			continue
		}
		info, err = dent.Info()
		if err != nil {
			t.Fatal(err)
		}
	}
	if info == nil {
		t.Fatal("missing dirent")
	}
	id, ok := IdentityFromInfo(info)
	opened, err := PathIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if ok && !id.Equal(opened) {
		t.Fatal("enumeration identity must match a real handle")
	}
	if !ok && (id.FileSystem != 0 || id.File != 0) {
		t.Fatal("missing identity must stay zero")
	}
	_ = HiddenFromInfo(path, info)
	if info.Size() != 1 {
		t.Fatalf("size %d", info.Size())
	}
}
