package filetypes

import (
	"crypto/sha256"
	"testing"
)

func TestDefaultsWithGlobsMatches(t *testing.T) {
	types := Defaults()
	if types.Len() < 265 {
		t.Fatalf("catalog %d", types.Len())
	}
	if types.Active() {
		t.Fatal("nothing selected")
	}
	names := types.Names()
	if len(names) < 265 || !types.Contains("go") {
		t.Fatal(names[:3])
	}
	custom := New().WithGlobs("notes", "*.md").Select("notes")
	if !custom.Active() {
		t.Fatal("active")
	}
	include, decided := custom.Matches("readme.md", "readme.md")
	if !decided || !include {
		t.Fatal("md")
	}
	include, decided = custom.Matches("a.go", "a.go")
	if !decided || include {
		t.Fatal("go")
	}
	custom.Negate("notes")
	if custom.IsEmpty() || !custom.Contains("notes") {
		t.Fatal("negate still defined")
	}
	composed := Defaults().WithComposedType("src", "go", "rust")
	if !composed.Contains("src") {
		t.Fatal(composed.Names())
	}
	h := sha256.New()
	types.WritePolicy(h)
	if h.Size() == 0 {
		t.Fatal("policy")
	}
	typed := New().WithType("go", ".go").Select("go")
	if ok, _ := typed.Matches("main.go", "main.go"); !ok {
		t.Fatal("withtype")
	}
}

func TestNilNamedTypes(t *testing.T) {
	var tpe *NamedFileTypes
	if tpe.Len() != 0 || !tpe.IsEmpty() || tpe.Active() || tpe.Contains("go") {
		t.Fatal("nil")
	}
	if tpe.Names() != nil {
		t.Fatal("names")
	}
	tpe.WritePolicy(sha256.New())
	neg := New().WithGlobs("notes", "*.md").Select("notes").Negate("notes")
	neg.WritePolicy(sha256.New())
}
