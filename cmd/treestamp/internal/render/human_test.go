package render

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteCardSkipsEmptyRows(t *testing.T) {
	var buf bytes.Buffer
	err := WriteCard(&buf, Palette{}, Card{
		Status: "Kept",
		Detail: "main.go",
		Rows: []Row{
			{Key: "Rule", Value: ""},
			{Key: "File", Value: ""},
			{Key: "Why", Value: "ignore_rule"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	if strings.Contains(text, "Rule") || strings.Contains(text, "File") {
		t.Fatalf("empty rows printed: %q", text)
	}
	if !strings.Contains(text, "Kept") || !strings.Contains(text, "Why") {
		t.Fatalf("%q", text)
	}
}

func TestPreviewCapsList(t *testing.T) {
	got := Preview([]string{"a", "b", "c"}, 2)
	if len(got) != 3 || got[2] != "… 1 more" {
		t.Fatalf("%q", got)
	}
}
