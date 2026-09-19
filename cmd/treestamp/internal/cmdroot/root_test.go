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
	if strings.Contains(text, "--quiet") {
		t.Fatalf("quiet still advertised %q", text)
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
	if !strings.Contains(text, "Complete") || !strings.Contains(text, "a.go") {
		t.Fatalf("%s", text)
	}
	if !strings.Contains(text, "1 file selected and hashed") {
		t.Fatalf("%s", text)
	}
	for _, junk := range []string{
		"within selected scope", "Policy exclusions", "repo-v1", "Failures",
		"Save a baseline", "Revision", "1 files",
	} {
		if strings.Contains(text, junk) {
			t.Fatalf("leaked %q: %s", junk, text)
		}
	}
}

func TestScanHumanListsNamesBeforeDropped(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "note.txt"), "hello\n")
	writeFile(t, filepath.Join(root, "payload.bin"), "a\x00b")
	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{"scan", root}, bytes.NewReader(nil), &out, &errb)
	if code != 0 {
		t.Fatalf("scan %d %s", code, errb.String())
	}
	text := out.String()
	note, dropped, bin := strings.Index(text, "note.txt"), strings.Index(text, "Dropped"), strings.Index(text, "payload.bin")
	if note < 0 || dropped < 0 || bin < 0 || note > dropped || dropped > bin {
		t.Fatalf("names under dropped: %s", text)
	}
}

func TestExplainMissingWouldKeep(t *testing.T) {
	root := t.TempDir()
	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{"explain", "nope.go", "--root", root}, bytes.NewReader(nil), &out, &errb)
	if code != 0 {
		t.Fatalf("explain %d %s", code, errb.String())
	}
	text := out.String()
	if !strings.Contains(text, "Would keep") || !strings.Contains(text, "not on disk") {
		t.Fatalf("%s", text)
	}
	if strings.Contains(text, "Kept") {
		t.Fatalf("said kept: %s", text)
	}
}

func TestNonScanHelpOmitsScanFlags(t *testing.T) {
	for _, name := range []string{"paths", "explain", "config"} {
		var out, errb bytes.Buffer
		if code := Run(context.Background(), []string{name, "--help"}, bytes.NewReader(nil), &out, &errb); code != 0 {
			t.Fatalf("%s help %d %s", name, code, errb.String())
		}
		text := out.String()
		if name == "paths" && !strings.Contains(text, "Binary and size checks") {
			t.Fatalf("missing honesty %q", text)
		}
		for _, junk := range []string{"--output", "--metadata-only", "--quiet", "ndjson"} {
			if strings.Contains(text, junk) {
				t.Fatalf("%s leaked %q: %s", name, junk, text)
			}
		}
	}
}

func TestConfigHumanHidesOracleIgnore(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Run(context.Background(), []string{"config"}, bytes.NewReader(nil), &out, &errb); code != 0 {
		t.Fatalf("config %d %s", code, errb.String())
	}
	text := out.String()
	if strings.Contains(text, "weavatrix") {
		t.Fatalf("oracle leak %s", text)
	}
	if !strings.Contains(text, ".gitignore") {
		t.Fatalf("%s", text)
	}
	if strings.Contains(text, "Hash contents") {
		t.Fatalf("default hash row %s", text)
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

func TestVerifyHumanListsEveryChangedPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep.go"), "package keep\n")
	base := filepath.Join(t.TempDir(), "base.tstamp.json")
	var out, errb bytes.Buffer
	if code := Run(context.Background(), []string{"scan", root, "--json", "--output", base}, bytes.NewReader(nil), &out, &errb); code != 0 {
		t.Fatalf("scan %d %s", code, errb.String())
	}
	for i := 0; i < 10; i++ {
		writeFile(t, filepath.Join(root, "extra"+itoa(i)+".go"), "package extra\n")
	}
	out.Reset()
	errb.Reset()
	code := Run(context.Background(), []string{"verify", base, "--root", root}, bytes.NewReader(nil), &out, &errb)
	if code != 1 {
		t.Fatalf("verify %d %s", code, errb.String())
	}
	text := out.String()
	if strings.Contains(text, "…") {
		t.Fatalf("truncated %s", text)
	}
	for i := 0; i < 10; i++ {
		name := "+ extra" + itoa(i) + ".go"
		if !strings.Contains(text, name) {
			t.Fatalf("missing %s in %s", name, text)
		}
	}
}

func TestVerifyNullWritesRecords(t *testing.T) {
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
	code := Run(context.Background(), []string{"verify", base, "--root", root, "--null"}, bytes.NewReader(nil), &out, &errb)
	if code != 1 {
		t.Fatalf("verify %d %s", code, errb.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("new.go\x00")) {
		t.Fatalf("%q", out.Bytes())
	}
	if bytes.Contains(out.Bytes(), []byte("+ new.go")) {
		t.Fatalf("human prefix in null output %q", out.Bytes())
	}
	if bytes.Contains(out.Bytes(), []byte("\n")) {
		t.Fatalf("newlines in null output %q", out.Bytes())
	}
}

func TestVerifyJSONAndNullIsUsage(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.go"), "package a\n")
	base := filepath.Join(t.TempDir(), "base.tstamp.json")
	var out, errb bytes.Buffer
	if code := Run(context.Background(), []string{"scan", root, "--json", "--output", base}, bytes.NewReader(nil), &out, &errb); code != 0 {
		t.Fatalf("scan %d %s", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := Run(context.Background(), []string{"verify", base, "--root", root, "--json", "--null"}, bytes.NewReader(nil), &out, &errb); code != 2 {
		t.Fatalf("code %d %s", code, errb.String())
	}
}

func TestScanNDJSONEmitsBeginFilesEnd(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.go"), "package a\n")
	writeFile(t, filepath.Join(root, "b.go"), "package b\n")
	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{"scan", root, "--ext", "go", "--format", "ndjson"}, bytes.NewReader(nil), &out, &errb)
	if code != 0 {
		t.Fatalf("scan %d %s", code, errb.String())
	}
	lines := bytes.Split(bytes.TrimRight(out.Bytes(), "\n"), []byte("\n"))
	if len(lines) != 4 {
		t.Fatalf("lines %d %s", len(lines), out.String())
	}
	var first, last map[string]any
	if err := json.Unmarshal(lines[0], &first); err != nil || first["event"] != "scan_begin" {
		t.Fatalf("begin %v %s", err, lines[0])
	}
	if err := json.Unmarshal(lines[len(lines)-1], &last); err != nil || last["event"] != "scan_end" {
		t.Fatalf("end %v %s", err, lines[len(lines)-1])
	}
	if last["complete"] != true {
		t.Fatalf("complete %v", last["complete"])
	}
	for _, line := range lines[1 : len(lines)-1] {
		var ev map[string]any
		if err := json.Unmarshal(line, &ev); err != nil || ev["event"] != "file_committed" {
			t.Fatalf("file %v %s", err, line)
		}
	}
}

func TestScanArtifactHashesBinaryAndGitignored(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".gitignore"), "*.bin\n")
	writeFile(t, filepath.Join(root, "note.txt"), "hello\n")
	writeFile(t, filepath.Join(root, "keep.bin"), "plain\n")
	writeFile(t, filepath.Join(root, "payload.bin"), "a\x00b")
	writeFile(t, filepath.Join(root, "node_modules", "pkg.js"), "module.exports=1\n")
	repo := scanJSON(t, root)
	if hasRel(repo, "keep.bin") || hasRel(repo, "payload.bin") || hasRel(repo, "node_modules/pkg.js") || !hasRel(repo, "note.txt") {
		t.Fatalf("repo files %+v", rels(repo))
	}
	art := scanJSON(t, root, "--profile", "artifact")
	if art.Policy.Profile != "artifact-v2" {
		t.Fatalf("profile %q", art.Policy.Profile)
	}
	if !hasHash(art, "keep.bin") || !hasHash(art, "payload.bin") || !hasHash(art, "note.txt") || !hasHash(art, "node_modules/pkg.js") {
		t.Fatalf("artifact files %+v", art.Files)
	}
}

func TestScanArtifactRejectsMetadataOnly(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.go"), "package a\n")
	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{"scan", root, "--profile", "artifact", "--metadata-only"}, bytes.NewReader(nil), &out, &errb)
	if code != 2 {
		t.Fatalf("code %d %s", code, errb.String())
	}
}

func TestVerifyArtifactRestoresBinaryHash(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "payload.bin"), "a\x00b")
	base := filepath.Join(t.TempDir(), "base.tstamp.json")
	var out, errb bytes.Buffer
	args := []string{"scan", root, "--profile", "artifact", "--json", "--output", base}
	if code := Run(context.Background(), args, bytes.NewReader(nil), &out, &errb); code != 0 {
		t.Fatalf("scan %d %s", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	code := Run(context.Background(), []string{"verify", base, "--root", root, "--json"}, bytes.NewReader(nil), &out, &errb)
	if code != 0 {
		t.Fatalf("verify %d out=%s err=%s", code, out.String(), errb.String())
	}
}

func scanJSON(t *testing.T, root string, extra ...string) store.Manifest {
	t.Helper()
	var out, errb bytes.Buffer
	args := append([]string{"scan", root, "--json"}, extra...)
	if code := Run(context.Background(), args, bytes.NewReader(nil), &out, &errb); code != 0 {
		t.Fatalf("scan %d %s", code, errb.String())
	}
	var man store.Manifest
	if err := json.Unmarshal(out.Bytes(), &man); err != nil {
		t.Fatal(err)
	}
	return man
}

func hasRel(man store.Manifest, name string) bool {
	for _, file := range man.Files {
		if file.Relative == name {
			return true
		}
	}
	return false
}

func hasHash(man store.Manifest, name string) bool {
	for _, file := range man.Files {
		if file.Relative == name && file.Hash != "" {
			return true
		}
	}
	return false
}

func rels(man store.Manifest) []string {
	out := make([]string, 0, len(man.Files))
	for _, file := range man.Files {
		out = append(out, file.Relative)
	}
	return out
}

func itoa(n int) string {
	return string(rune('0' + n))
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
