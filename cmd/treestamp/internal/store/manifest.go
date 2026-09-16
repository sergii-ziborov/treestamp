package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/sergii-ziborov/treestamp"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/policy"
)

const Schema = "treestamp.manifest/v1"

type Manifest struct {
	Schema        string          `json:"schema"`
	Producer      Producer        `json:"producer"`
	Policy        policy.Snapshot `json:"policy"`
	Observation   Observation     `json:"observation"`
	Files         []File          `json:"files"`
	IgnoreSources []IgnoreSource  `json:"ignore_sources,omitempty"`
	Revisions     Revisions       `json:"revisions"`
	Summary       Summary         `json:"summary"`
}

type IgnoreSource struct {
	Kind     treestamp.IgnoreSourceKind `json:"kind"`
	Location string                     `json:"location,omitempty"`
	Hash     string                     `json:"content_hash"`
}

type Producer struct {
	CLI    string `json:"cli"`
	Core   string `json:"core"`
	Schema string `json:"schema"`
}

type Observation struct {
	Complete    bool   `json:"complete"`
	Termination string `json:"termination,omitempty"`
	Portable    bool   `json:"portable"`
	Evidence    string `json:"evidence"`
}

type File struct {
	Relative string `json:"relative"`
	RawB64   string `json:"raw_b64,omitempty"`
	Bytes    uint64 `json:"bytes"`
	Hash     string `json:"sha256,omitempty"`
}

type Revisions struct {
	Legacy string `json:"legacy,omitempty"`
}

type Summary struct {
	Selected int `json:"selected"`
	Hashed   int `json:"hashed"`
	Excluded int `json:"excluded"`
	Failures int `json:"failures"`
}

func FromReport(rep *treestamp.ScanReport, snap policy.Snapshot) Manifest {
	out := Manifest{
		Schema:   Schema,
		Producer: Producer{CLI: policy.CLIVersion, Core: treestamp.Version, Schema: Schema},
		Policy:   snap,
		Observation: Observation{
			Complete: rep.Complete, Termination: rep.Termination.String(),
			Portable: rep.Portable, Evidence: evidenceOf(rep),
		},
		Revisions: Revisions{Legacy: rep.Revision},
	}
	sum := rep.Summary()
	out.Summary = Summary{Selected: sum.SelectedFiles, Hashed: sum.HashedFiles, Excluded: sum.RecordedSkips}
	for _, file := range rep.Files {
		out.Files = append(out.Files, encodeFile(file))
	}
	for _, src := range rep.ToPortable().IgnoreSources {
		out.IgnoreSources = append(out.IgnoreSources, IgnoreSource{
			Kind: src.Kind, Location: src.RepositoryRelative, Hash: src.ContentHash,
		})
	}
	for _, skipped := range rep.Skipped {
		if skipped.Kind == treestamp.SkipIOError || skipped.Kind == treestamp.SkipConcurrentModification {
			out.Summary.Failures++
		}
	}
	return out
}

func encodeFile(file treestamp.ScannedFile) File {
	item := File{Bytes: file.Bytes, Hash: file.ContentHash}
	if utf8.ValidString(file.Relative) {
		item.Relative = file.Relative
		return item
	}
	item.RawB64 = file.Relative
	item.Relative = strings.ToValidUTF8(file.Relative, "\uFFFD")
	return item
}

func evidenceOf(rep *treestamp.ScanReport) string {
	if rep.Summary().HashedFiles > 0 {
		return "sha256"
	}
	return "metadata"
}

func (m Manifest) AsReport() *treestamp.ScanReport {
	files := make([]treestamp.ScannedFile, 0, len(m.Files))
	for _, file := range m.Files {
		rel := file.Relative
		if file.RawB64 != "" {
			rel = file.RawB64
		}
		files = append(files, treestamp.ScannedFile{Relative: rel, Bytes: file.Bytes, ContentHash: file.Hash})
	}
	sources := make([]treestamp.IgnoreSourceEvidence, 0, len(m.IgnoreSources))
	for _, src := range m.IgnoreSources {
		sources = append(sources, treestamp.IgnoreSourceEvidence{
			Kind: src.Kind, Location: src.Location, ContentHash: src.Hash,
		})
	}
	return &treestamp.ScanReport{
		Files: files, Revision: m.Revisions.Legacy, Complete: m.Observation.Complete,
		Portable: m.Observation.Portable, IgnoreSources: sources,
		Descriptor: treestamp.ScanDescriptor{
			Version: m.Policy.Descriptor.Version, Policy: m.Policy.Descriptor.Policy,
		},
	}
}

func Load(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	if len(data) > 64<<20 {
		return Manifest{}, fmt.Errorf("manifest exceeds 64MiB read limit")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, err
	}
	if err := m.validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func (m Manifest) validate() error {
	if m.Schema != Schema {
		return fmt.Errorf("unsupported manifest schema %q", m.Schema)
	}
	seen := map[string]struct{}{}
	for _, file := range m.Files {
		rel := file.Relative
		if unsafePath(rel) {
			return fmt.Errorf("unsafe path %q", rel)
		}
		if _, ok := seen[rel]; ok {
			return fmt.Errorf("duplicate path %q", rel)
		}
		seen[rel] = struct{}{}
	}
	return nil
}

func unsafePath(rel string) bool {
	if rel == "" || strings.ContainsRune(rel, 0) || strings.HasPrefix(rel, "/") || strings.Contains(rel, ":") {
		return true
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}
