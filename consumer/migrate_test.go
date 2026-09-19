package consumer_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fastwalk "github.com/sergii-ziborov/treestamp/compat/fastwalk"
)

func TestFastwalkImportWalksTree(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "keep.go"), []byte("package keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var names []string
	err := fastwalk.Walk(nil, root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		names = append(names, d.Name())
		return nil
	})
	if err != nil || len(names) != 1 || names[0] != "keep.go" {
		t.Fatalf("%v %v", names, err)
	}
}

func TestConsumerGoModHasNoCompetitor(t *testing.T) {
	mod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	text := string(mod)
	for _, dep := range []string{
		"github.com/charlievieth/fastwalk",
		"github.com/karrick/godirwalk",
		"github.com/boyter/gocodewalker",
		"github.com/fsnotify/fsnotify",
	} {
		if strings.Contains(text, dep) {
			t.Fatalf("consumer require %s", dep)
		}
	}
}
