package cmdroot

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/store"
)

func TestHelpAndVersionDoNotScan(t *testing.T) {
	var out, err bytes.Buffer
	if code := Run(context.Background(), nil, bytes.NewReader(nil), &out, &err); code != 0 {
		t.Fatalf("help %d %s", code, err.String())
	}
	help := out.String()
	if !strings.Contains(help, "scan") || !strings.Contains(help, "verify") {
		t.Fatalf("help %q", help)
	}
	for _, junk := range []string{"completion", "This is not find", "DOCTOR", "repo-v1"} {
		if strings.Contains(help, junk) {
			t.Fatalf("help leaked %q: %s", junk, help)
		}
	}
	out.Reset()
	if code := Run(context.Background(), []string{"version", "--json"}, bytes.NewReader(nil), &out, &err); code != 0 {
		t.Fatalf("version %d %s", code, err.String())
	}
	if !strings.Contains(out.String(), `"cli"`) || strings.Contains(out.String(), "\x1b") {
		t.Fatalf("version json %q", out.String())
	}
}

func TestScanHelpShowsCommittedBaseline(t *testing.T) {
	var out, err bytes.Buffer
	if code := Run(context.Background(), []string{"scan", "--help"}, bytes.NewReader(nil), &out, &err); code != 0 {
		t.Fatalf("help %d %s", code, err.String())
	}
	text := out.String()
	if !strings.Contains(text, "--output ./baselines/cli.tstamp.json") {
		t.Fatalf("missing committed-baseline example %q", text)
	}
	if !strings.Contains(text, "../repo.tstamp.json") {
		t.Fatalf("missing sibling-baseline example %q", text)
	}
}

func TestUnknownCommandIsUsage(t *testing.T) {
	var out, err bytes.Buffer
	if code := Run(context.Background(), []string{"not-a-command"}, bytes.NewReader(nil), &out, &err); code != 2 {
		t.Fatalf("code %d err=%q", code, err.String())
	}
}

func TestScanJSONAndOutputOutsideRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.go"), "package a\n")
	outPath := filepath.Join(t.TempDir(), "baseline.tstamp.json")
	var out, errb bytes.Buffer
	args := []string{"scan", root, "--json", "--output", outPath}
	if code := Run(context.Background(), args, bytes.NewReader(nil), &out, &errb); code != 0 {
		t.Fatalf("scan %d %s", code, errb.String())
	}
	if bytes.Contains(out.Bytes(), []byte{0x1b}) {
		t.Fatal("ansi in json")
	}
	var man store.Manifest
	if err := json.Unmarshal(out.Bytes(), &man); err != nil {
		t.Fatal(err)
	}
	if man.Schema != store.Schema || man.Summary.Selected < 1 {
		t.Fatalf("%+v", man)
	}
	loaded, err := store.Load(outPath)
	if err != nil || loaded.Summary.Selected != man.Summary.Selected {
		t.Fatalf("saved %v %+v", err, loaded)
	}
}

func TestScanRefusesOutputInsideRoot(t *testing.T) {
	root := t.TempDir()
	var out, errb bytes.Buffer
	inside := filepath.Join(root, "baseline.tstamp.json")
	code := Run(context.Background(), []string{"scan", root, "--json", "--output", inside}, bytes.NewReader(nil), &out, &errb)
	if code != 2 {
		t.Fatalf("code %d %s", code, errb.String())
	}
	if _, err := os.Stat(inside); !os.IsNotExist(err) {
		t.Fatal("wrote inside root")
	}
}

func TestVerifyMatchUnchanged(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "old.go"), "package old\n")
	base := filepath.Join(t.TempDir(), "base.tstamp.json")
	var out, errb bytes.Buffer
	if code := Run(context.Background(), []string{"scan", root, "--json", "--output", base}, bytes.NewReader(nil), &out, &errb); code != 0 {
		t.Fatalf("scan %d %s", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	code := Run(context.Background(), []string{"verify", base, "--root", root, "--json"}, bytes.NewReader(nil), &out, &errb)
	if code != 0 {
		t.Fatalf("verify match %d out=%s err=%s", code, out.String(), errb.String())
	}
}

func TestVerifyDetectsNewSelectedFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "old.go"), "package old\n")
	base := filepath.Join(t.TempDir(), "base.tstamp.json")
	var out, errb bytes.Buffer
	if code := Run(context.Background(), []string{"scan", root, "--json", "--output", base}, bytes.NewReader(nil), &out, &errb); code != 0 {
		t.Fatalf("scan %d %s", code, errb.String())
	}
	writeFile(t, filepath.Join(root, "new.go"), "package neu\n")
	out.Reset()
	errb.Reset()
	code := Run(context.Background(), []string{"verify", base, "--root", root, "--json"}, bytes.NewReader(nil), &out, &errb)
	if code != 1 {
		t.Fatalf("verify code %d out=%s err=%s", code, out.String(), errb.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("new.go")) {
		t.Fatalf("missing new file %s", out.String())
	}
}

func TestPathsEmptyIsSuccess(t *testing.T) {
	root := t.TempDir()
	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{"paths", root, "--ext", "rs"}, bytes.NewReader(nil), &out, &errb)
	if code != 0 {
		t.Fatalf("paths %d %s", code, errb.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected empty listing %q", out.String())
	}
}

func TestExplainSelectedOmitsEmptyRows(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep.go"), "package keep\n")
	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{"explain", "keep.go", "--root", root, "--ext", "go"}, bytes.NewReader(nil), &out, &errb)
	if code != 0 {
		t.Fatalf("explain %d %s", code, errb.String())
	}
	text := out.String()
	if !strings.Contains(text, "Kept") || !strings.Contains(text, "keep.go") {
		t.Fatalf("%s", text)
	}
	for _, junk := range []string{"Reason", "Not checked", "content bytes", "selected"} {
		if strings.Contains(text, junk) {
			t.Fatalf("leaked %q: %s", junk, text)
		}
	}
}

func TestScanHumanOmitsLecture(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.go"), "package a\n")
	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{"scan", root, "--ext", "go"}, bytes.NewReader(nil), &out, &errb)
	if code != 0 {
		t.Fatalf("scan %d %s", code, errb.String())
	}
	text := out.String()
	if !strings.Contains(text, "Complete") {
		t.Fatalf("%s", text)
	}
	for _, junk := range []string{"within selected scope", "Policy exclusions", "repo-v1", "Failures"} {
		if strings.Contains(text, junk) {
			t.Fatalf("leaked %q: %s", junk, text)
		}
	}
}

func TestExplainExcludedIsSuccess(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".gitignore"), "skip.txt\n")
	writeFile(t, filepath.Join(root, "skip.txt"), "x")
	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{"explain", "skip.txt", "--root", root}, bytes.NewReader(nil), &out, &errb)
	if code != 0 {
		t.Fatalf("explain %d %s", code, errb.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("skip.txt")) {
		t.Fatalf("%s", out.String())
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
