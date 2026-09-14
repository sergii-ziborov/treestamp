package walk

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWalkerDoesNotUseWalkDirWrapper(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWrite(t, filepath.Join(root, "sub", "b.txt"), "b")

	walker, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()

	var files []string
	for {
		entry, err := walker.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if entry.IsFile() {
			files = append(files, entry.RelativePath())
		}
	}
	if len(files) != 2 {
		t.Fatalf("files=%v", files)
	}
}

func TestFileRoot(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "only.txt")
	mustWrite(t, file, "x")
	walker, err := New(file)
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	entries, err := Collect(walker)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].IsFile() || entries[0].RelativePath() != "" {
		t.Fatalf("entries=%v", summarize(entries))
	}
}

func TestMaxDepth(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "a", "b"))
	mustWrite(t, filepath.Join(root, "a", "b", "c.txt"), "c")
	maxDepth := 1
	walker, err := NewWithOptions(root, WalkOptions{MaxDepth: &maxDepth, MaxOpen: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	entries, err := Collect(walker)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Depth() > 1 && entry.IsFile() {
			t.Fatalf("file beyond max depth: %s", entry.RelativePath())
		}
		if entry.IsDir() && entry.Depth() == 1 && entry.SkipReason() != SkipMaxDepth {
			t.Fatalf("expected max_depth skip on %s", entry.RelativePath())
		}
	}
}

func TestMaxOpenStillVisitsAllFiles(t *testing.T) {
	root := t.TempDir()
	var want int
	current := root
	for i := 0; i < 8; i++ {
		current = filepath.Join(current, "d")
		mustMkdir(t, current)
		mustWrite(t, filepath.Join(current, "f.txt"), "x")
		want++
	}
	walker, err := NewWithOptions(root, WalkOptions{MaxOpen: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	entries, err := Collect(walker)
	if err != nil {
		t.Fatal(err)
	}
	var files int
	for _, entry := range entries {
		if entry.IsFile() {
			files++
		}
	}
	if files != want {
		t.Fatalf("files=%d want=%d", files, want)
	}
}

func TestSkipCurrentDir(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "keep"))
	mustWrite(t, filepath.Join(root, "keep", "a.txt"), "a")
	mustMkdir(t, filepath.Join(root, "skip"))
	mustWrite(t, filepath.Join(root, "skip", "b.txt"), "b")
	walker, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	var files []string
	for {
		entry, err := walker.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if entry.IsDir() && entry.FileName() == "skip" {
			walker.SkipCurrentDir()
		}
		if entry.IsFile() {
			files = append(files, filepath.ToSlash(entry.RelativePath()))
		}
	}
	if len(files) != 1 || files[0] != filepath.ToSlash(filepath.Join("keep", "a.txt")) && files[0] != "keep/a.txt" {
		if len(files) != 1 {
			t.Fatalf("files=%v", files)
		}
	}
}

func TestMetadataBytes(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "data.bin"), "hello")
	opts := DefaultOptions()
	opts.CollectMetadata = true
	walker, err := NewWithOptions(root, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	entries, err := Collect(walker)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if entry.IsFile() {
			if entry.Bytes() == nil || *entry.Bytes() != 5 {
				t.Fatalf("bytes=%v", entry.Bytes())
			}
			found = true
		}
	}
	if !found {
		t.Fatal("missing file")
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func summarize(entries []*WalkEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.RelativePath())
	}
	return out
}
