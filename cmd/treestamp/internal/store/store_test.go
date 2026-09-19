package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sergii-ziborov/treestamp"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/policy"
)

func TestInsideRootAndAtomicReplace(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "out.json")
	ok, err := InsideRoot(root, inside)
	if err != nil || !ok {
		t.Fatalf("inside %v %v", ok, err)
	}
	outside := filepath.Join(t.TempDir(), "out.json")
	ok, err = InsideRoot(root, outside)
	if err != nil || ok {
		t.Fatalf("outside %v %v", ok, err)
	}
	if err := WriteAtomic(outside, []byte("one\n")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(outside, []byte("two\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "two\n" {
		t.Fatalf("%q %v", data, err)
	}
}

func TestEncodeNativeB64AndEmptyHashed(t *testing.T) {
	raw := string([]byte{0xff, 0xfe, 'a'})
	item := encodeFile(treestamp.ScannedFile{
		Relative: raw, Bytes: 1, ContentHash: "sha256:" + strings.Repeat("ab", 32),
	})
	if item.RawB64 == "" || item.RawB64 == raw {
		t.Fatalf("raw %q", item.RawB64)
	}
	got, err := decodeRelative(item)
	if err != nil || got != raw {
		t.Fatalf("%q %v", got, err)
	}
	man := FromReport(&treestamp.ScanReport{Complete: true}, policy.Snapshot{HashContents: true})
	if man.Observation.Evidence != "sha256" {
		t.Fatalf("evidence %s", man.Observation.Evidence)
	}
	if err := man.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadTrailingAndMissingHash(t *testing.T) {
	m := Manifest{Schema: Schema, Observation: Observation{Evidence: "sha256"}, Files: []File{{Relative: "a.go"}}}
	if err := m.validate(); err == nil {
		t.Fatal("missing hash")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "m.json")
	body := `{"schema":"treestamp.manifest/v1","producer":{},"policy":{},"observation":{"evidence":"sha256"},"files":[],"revisions":{},"summary":{}} extra`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("trailing")
	}
}

func TestRejectsDuplicateAndDotDot(t *testing.T) {
	m := Manifest{Schema: Schema, Files: []File{{Relative: "a/../b"}}}
	if err := m.validate(); err == nil {
		t.Fatal("dotdot")
	}
	m = Manifest{Schema: Schema, Files: []File{{Relative: "a.go"}, {Relative: "a.go"}}}
	if err := m.validate(); err == nil {
		t.Fatal("dup")
	}
}

func TestLoadRequiresDocumentEnd(t *testing.T) {
	base := `{"schema":"treestamp.manifest/v1","producer":{},"policy":{},"observation":{"evidence":"sha256"},"files":[],"revisions":{},"summary":{}}`
	if _, err := loadManifest(t, base+"  \n\t"); err != nil {
		t.Fatalf("whitespace eof: %v", err)
	}
	empty := `{"schema":"treestamp.manifest/v1","producer":{},"policy":{},"observation":{"complete":true,"evidence":"sha256"},"files":[],"revisions":{},"summary":{}}`
	if _, err := loadManifest(t, empty); err != nil {
		t.Fatalf("empty hashed: %v", err)
	}
	for _, tail := range []string{"]", "}", "] leftover", "{}", "1", "null", "xxx", "\xff"} {
		if _, err := loadManifest(t, base+tail); err == nil {
			t.Fatalf("accepted trailing %q", tail)
		}
	}
}

func TestLoadRejectsContradictoryObservation(t *testing.T) {
	cases := []string{
		`{"schema":"treestamp.manifest/v1","producer":{},"policy":{},"observation":{"complete":true,"termination":"timeout","evidence":"sha256"},"files":[],"revisions":{},"summary":{}}`,
		`{"schema":"treestamp.manifest/v1","producer":{},"policy":{},"observation":{"complete":true,"termination":"cancelled","evidence":"sha256"},"files":[],"revisions":{},"summary":{}}`,
		`{"schema":"treestamp.manifest/v1","producer":{},"policy":{},"observation":{"complete":true,"evidence":"sha256"},"files":[],"revisions":{},"summary":{"failures":1}}`,
		`{"schema":"treestamp.manifest/v1","producer":{},"policy":{},"observation":{"evidence":"md5"},"files":[],"revisions":{},"summary":{}}`,
		`{"schema":"treestamp.manifest/v1","producer":{},"policy":{"profile":"artifact-v1"},"observation":{"evidence":"sha256"},"files":[],"revisions":{},"summary":{}}`,
		`{"schema":"treestamp.manifest/v1","producer":{},"policy":{},"observation":{"evidence":"sha256"},"files":[],"revisions":{},"summary":{"selected":2}}`,
	}
	for _, body := range cases {
		if _, err := loadManifest(t, body); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestAsReportKeepsTermination(t *testing.T) {
	man := Manifest{
		Schema: Schema, Observation: Observation{Termination: "timeout", Evidence: "sha256"},
	}
	got := man.AsReport()
	if got.Termination != treestamp.TerminationTimeout {
		t.Fatalf("term %v", got.Termination)
	}
}

func loadManifest(t *testing.T, body string) (Manifest, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "m.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}
