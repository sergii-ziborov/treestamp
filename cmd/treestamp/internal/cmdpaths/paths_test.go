package cmdpaths

import (
	"path/filepath"
	"testing"
)

func TestAbsPathsResolvesRoot(t *testing.T) {
	got, err := absPaths(".", []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !filepath.IsAbs(got[0]) {
		t.Fatalf("%q", got)
	}
}
