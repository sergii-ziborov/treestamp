package report

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sergii-ziborov/treestamp/internal/fileread"
	"github.com/sergii-ziborov/treestamp/internal/hashx"
)

func TestHashTextAndLocations(t *testing.T) {
	if HashText("x") == HashText("y") || HashText("") == "" {
		t.Fatal("hash")
	}
	if RepoRelativeLocation("<git>") != "<git>" {
		t.Fatal("bracket")
	}
	if RepoRelativeLocation("/abs/path") != "" || RepoRelativeLocation("C:\\abs") != "" {
		t.Fatal("abs")
	}
	if RepoRelativeLocation("src\\a.go") != "src/a.go" {
		t.Fatal("slash")
	}
	if !LooksAbsolute("/") || !LooksAbsolute("\\windows") {
		t.Fatal("looks")
	}
}

func TestDeltaAddRemoveModifyRename(t *testing.T) {
	prev := []Record{
		{Relative: "keep.txt", ContentHash: "h1", Bytes: 1},
		{Relative: "old.txt", ContentHash: "gone", Bytes: 2},
		{Relative: "edit.txt", ContentHash: "a", Bytes: 3},
		{Relative: "moved.txt", ContentHash: "mv", Bytes: 4},
	}
	cur := []Record{
		{Relative: "keep.txt", ContentHash: "h1", Bytes: 1},
		{Relative: "new.txt", ContentHash: "n", Bytes: 2},
		{Relative: "edit.txt", ContentHash: "b", Bytes: 3},
		{Relative: "renamed.txt", ContentHash: "mv", Bytes: 4},
	}
	d := Between(prev, cur)
	if d.Unchanged != 1 || len(d.Added) != 1 || len(d.Removed) != 1 || len(d.Modified) != 1 || len(d.Renamed) != 1 {
		t.Fatalf("%+v", d)
	}
	if d.Added[0].Relative != "new.txt" || d.Removed[0].Relative != "old.txt" {
		t.Fatalf("paths %+v", d)
	}
	if d.Renamed[0].Previous.Relative != "moved.txt" || d.Renamed[0].Current.Relative != "renamed.txt" {
		t.Fatalf("rename %+v", d.Renamed)
	}
	if SameContent(Record{Bytes: 1}, Record{Bytes: 2}) {
		t.Fatal("bytes")
	}
	if !SameContent(Record{Bytes: 1, VersionKey: "v"}, Record{Bytes: 1, VersionKey: "v"}) {
		t.Fatal("version")
	}
}

func TestDeltaDuplicateHashesStayAdded(t *testing.T) {
	prev := []Record{{Relative: "a", ContentHash: "dup", Bytes: 1}, {Relative: "b", ContentHash: "dup", Bytes: 1}}
	cur := []Record{{Relative: "c", ContentHash: "dup", Bytes: 1}, {Relative: "d", ContentHash: "dup", Bytes: 1}}
	d := Between(prev, cur)
	if len(d.Renamed) != 0 || len(d.Added) != 2 || len(d.Removed) != 2 {
		t.Fatalf("dup hashes %+v", d)
	}
}

func TestSnapshotValidateAndRead(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash := hashx.SHA256Prefix([]byte("abc"))
	files := []File{{Absolute: path, Relative: "a.txt", ContentHash: hash, Bytes: 3}}
	if err := Validate(root, files); err != nil {
		t.Fatal(err)
	}
	if _, ok := Lookup(files, "missing"); ok {
		t.Fatal("missing")
	}
	data, ev, err := ReadBounded(root, files, "a.txt", 10)
	if err != nil || string(data) != "abc" || ev != EvidenceSHA256 {
		t.Fatalf("%q %d %v", data, ev, err)
	}
	if _, _, err := ReadBounded(root, files, "a.txt", 1); err == nil {
		t.Fatal("limit")
	}
	if _, _, err := ReadBounded(root, files, "nope", 10); err == nil {
		t.Fatal("unknown")
	}
	if err := os.WriteFile(path, []byte("xyz"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadBounded(root, files, "a.txt", 10); err == nil {
		t.Fatal("stale hash")
	}
}

func TestSnapshotValidateErrors(t *testing.T) {
	if err := Validate("", nil); err == nil {
		t.Fatal("empty root")
	}
	if err := Validate("rel", nil); err == nil {
		t.Fatal("relative root")
	}
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root, []File{{Absolute: path, Relative: "z.txt", Bytes: 1, ContentHash: "h"}, {Absolute: path, Relative: "a.txt", Bytes: 1, ContentHash: "h"}}); err == nil {
		t.Fatal("unsorted")
	}
	if err := Validate(root, []File{{Absolute: "", Relative: "a.txt", Bytes: 1, ContentHash: "h"}}); err == nil {
		t.Fatal("empty abs")
	}
	if err := Validate(root, []File{{Absolute: path, Relative: "a.txt", Bytes: 1}}); err == nil {
		t.Fatal("no evidence")
	}
	outside := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root, []File{{Absolute: outside, Relative: "x", Bytes: 1, ContentHash: "h"}}); err == nil {
		t.Fatal("escape")
	}
	ns := uint64(1)
	ver := []File{{Absolute: path, Relative: "a.txt", Bytes: 1, ModifiedNS: &ns}}
	if err := Validate(root, ver); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotRejectsStaleVersionAndEscape(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	old := uint64(1)
	files := []File{{Absolute: path, Relative: "a.txt", Bytes: 3, ModifiedNS: &old}}
	if _, _, err := ReadBounded(root, files, "a.txt", 10); err == nil {
		t.Fatal("stale version accepted")
	}
	ns := uint64(info.ModTime().UnixNano())
	ok := []File{{Absolute: path, Relative: "a.txt", Bytes: 3, ModifiedNS: &ns}}
	if data, ev, err := ReadBounded(root, ok, "a.txt", 10); err != nil || string(data) != "abc" || ev != EvidenceFileVersion {
		t.Fatalf("version read %q %d %v", data, ev, err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "data.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "data.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	escaped := []File{{Absolute: filepath.Join(root, "sub", "data.txt"), Relative: "sub/data.txt", Bytes: 3, ContentHash: hashx.SHA256Prefix([]byte("abc"))}}
	if err := os.RemoveAll(filepath.Join(root, "sub")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "sub")); err != nil {
		t.Skip(err)
	}
	if _, _, err := ReadBounded(root, escaped, "sub/data.txt", 10); err == nil {
		t.Fatal("directory symlink escape accepted")
	}
}

func TestMapIOAndSlashRel(t *testing.T) {
	if err := mapIO("gone.txt", os.ErrNotExist); err == nil {
		t.Fatal("stale")
	}
	err := mapIO("x", os.ErrPermission)
	var typed *Error
	if !errors.As(err, &typed) || typed.Reason != "io" {
		t.Fatalf("%v", err)
	}
	if slashRel(".") != "" || slashRel("a/b") != "a/b" {
		t.Fatal("slash")
	}
}

func TestSnapshotIdentityRejectsReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("xxx"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	ver := fileread.FromInfo(path, info)
	files := []File{{
		Absolute: path, Relative: "a.txt", Bytes: 3,
		ContentHash: hashx.SHA256Prefix([]byte("xxx")),
		ModifiedNS:  ver.ModifiedNS, Identity: ver.Identity,
	}}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("yyy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadBounded(root, files, "a.txt", 100); err == nil {
		t.Fatal("same-size replacement accepted")
	}
}

func TestSnapshotErrorText(t *testing.T) {
	if (*Error)(nil).Error() != "snapshot read error" {
		t.Fatal("nil")
	}
	if (*Error)(nil).Unwrap() != nil {
		t.Fatal("unwrap nil")
	}
	cases := []Error{
		{Reason: "unknown", Relative: "a"},
		{Reason: "stale", Relative: "a"},
		{Reason: "limit", Relative: "a", Bytes: 2, MaxBytes: 1},
		{Reason: "invalid", Err: errors.New("x")},
		{Reason: "io", Relative: "a", Err: errors.New("x")},
		{Reason: "other", Relative: "a", Err: errors.New("x")},
		{Reason: "other", Relative: "a"},
	}
	for _, c := range cases {
		if c.Error() == "" {
			t.Fatalf("%+v", c)
		}
	}
	if !errors.Is(&Error{Err: os.ErrNotExist}, os.ErrNotExist) {
		t.Fatal("is")
	}
}
