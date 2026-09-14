package walk

import (
	"path/filepath"
	"testing"
)

func TestWalkBuilderSortAndMultiRoot(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	mustWrite(t, filepath.Join(a, "z.txt"), "z")
	mustWrite(t, filepath.Join(a, "a.txt"), "a")
	mustWrite(t, filepath.Join(b, "m.txt"), "m")

	walker := NewBuilder(a).AddRoot(b).SortByFileName().Build()
	defer walker.Close()
	entries, err := Collect(walker)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, entry := range entries {
		if entry.IsFile() {
			files = append(files, entry.FileName())
		}
	}
	if len(files) != 3 {
		t.Fatalf("files=%v", files)
	}
	if files[0] != "a.txt" || files[1] != "z.txt" || files[2] != "m.txt" {
		t.Fatalf("order=%v", files)
	}
}

func TestContentsFirst(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWrite(t, filepath.Join(root, "sub", "f.txt"), "f")
	walker := NewBuilder(root).ContentsFirst(true).Build()
	defer walker.Close()
	entries, err := Collect(walker)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		rel := filepath.ToSlash(entry.RelativePath())
		if rel == "" {
			rel = "."
		}
		names = append(names, rel)
	}
	if len(names) < 3 {
		t.Fatalf("names=%v", names)
	}
	if names[len(names)-1] != "." {
		t.Fatalf("root should be last in contents-first, got %v", names)
	}
}

func TestFilterPrunesDirectory(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "keep"))
	mustWrite(t, filepath.Join(root, "keep", "ok.txt"), "ok")
	mustMkdir(t, filepath.Join(root, "drop"))
	mustWrite(t, filepath.Join(root, "drop", "no.txt"), "no")
	walker := NewBuilder(root).FilterEntry(func(entry *WalkEntry) bool {
		return entry.FileName() != "drop"
	}).Build()
	defer walker.Close()
	entries, err := Collect(walker)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.FileName() == "no.txt" || entry.FileName() == "drop" {
			t.Fatalf("filter leaked %s", entry.Path())
		}
	}
}
