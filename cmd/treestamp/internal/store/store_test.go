package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInsideRootAndAtomicReplace(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "out.json")
	ok, err := InsideRoot(root, inside)
	if err != nil || !ok {
		t.Fatalf("inside %v %v", ok, err)
	}
	outside := filepath.Join(t.TempDir(), "out.json")
	ok, err = InsideRoot(root, outside)
	if err != nil || ok {
		t.Fatalf("outside %v %v", ok, err)
	}
	if err := WriteAtomic(outside, []byte("one\n")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(outside, []byte("two\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "two\n" {
		t.Fatalf("%q %v", data, err)
	}
}

func TestRejectsDuplicateAndDotDot(t *testing.T) {
	m := Manifest{Schema: Schema, Files: []File{{Relative: "a/../b"}}}
	if err := m.validate(); err == nil {
		t.Fatal("dotdot")
	}
	m = Manifest{Schema: Schema, Files: []File{{Relative: "a.go"}, {Relative: "a.go"}}}
	if err := m.validate(); err == nil {
		t.Fatal("dup")
	}
}
