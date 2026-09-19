package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sergii-ziborov/treestamp"
)

func TestDeltaLinesAndNull(t *testing.T) {
	d := OfDelta(treestamp.ScanDelta{
		Added:    []treestamp.ScannedFile{{Relative: "new.go"}},
		Removed:  []treestamp.ScannedFile{{Relative: "gone.go"}},
		Modified: []treestamp.ModifiedFile{{Current: treestamp.ScannedFile{Relative: "edit.go"}}},
		Renamed: []treestamp.RenamedFile{{
			Previous: treestamp.ScannedFile{Relative: "old.go"},
			Current:  treestamp.ScannedFile{Relative: "ren.go"},
		}},
	})
	lines := d.Lines()
	if len(lines) != 4 || lines[0] != "+ new.go" || lines[3] != "old.go → ren.go" {
		t.Fatalf("%q", lines)
	}
	var buf bytes.Buffer
	if err := WriteNull(&buf, lines); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 || strings.Contains(buf.String(), "\n") {
		t.Fatalf("%q", buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte("new.go\x00")) {
		t.Fatalf("%q", buf.Bytes())
	}
	paths := d.Paths()
	if len(paths) != 5 || paths[0] != "new.go" || paths[3] != "old.go" || paths[4] != "ren.go" {
		t.Fatalf("paths %q", paths)
	}
}
