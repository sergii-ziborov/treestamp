package policy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyConfigMaxFileBytesAndFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.json")
	if err := os.WriteFile(path, []byte(`{"schema":"treestamp.policy/v1","max_file_bytes":12}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sel := Select{Config: path, Format: "text", Color: "auto"}
	if err := sel.ApplyConfig(); err != nil {
		t.Fatal(err)
	}
	opts, err := sel.Options()
	if err != nil || opts.MaxFileBytes != 12 {
		t.Fatalf("max %d err %v", opts.MaxFileBytes, err)
	}
	bad := Select{Format: "xml", Color: "auto"}
	if err := bad.ApplyConfig(); err == nil {
		t.Fatal("unknown format")
	}
}
