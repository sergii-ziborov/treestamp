package fileread

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sergii-ziborov/treestamp/internal/dirread"
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

type listedDent struct {
	name string
	typ  os.FileMode
	info int
}

func (d *listedDent) Name() string               { return d.name }
func (d *listedDent) IsDir() bool                { return d.typ.IsDir() }
func (d *listedDent) Type() os.FileMode          { return d.typ }
func (d *listedDent) Info() (os.FileInfo, error) { d.info++; return nil, os.ErrNotExist }

func TestModeOfZeroTypeSkipsInfo(t *testing.T) {
	dent := &listedDent{name: "a.txt"}
	mode, err := dirread.ModeOf(dent)
	if err != nil || !mode.IsRegular() || dent.info != 0 {
		t.Fatalf("mode=%v err=%v info=%d", mode, err, dent.info)
	}
}

func TestModeOfUnknownTypeCallsInfo(t *testing.T) {
	dent := &listedDent{name: "?", typ: dirread.UnknownType}
	if _, err := dirread.ModeOf(dent); err == nil || dent.info != 1 {
		t.Fatalf("err=%v info=%d", err, dent.info)
	}
}
