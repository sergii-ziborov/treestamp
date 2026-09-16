// Command treestamp-driver speaks the Treestamp fixture protocol.
package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/sergii-ziborov/treestamp"
)

type request struct {
	Op      string          `json:"op"`
	Root    string          `json:"root"`
	Session string          `json:"session"`
	Options json.RawMessage `json:"options"`
	Plan    watchPlanJSON   `json:"plan"`
}

type watchPlanJSON struct {
	Changed        []string `json:"changed"`
	Removed        []string `json:"removed"`
	FullRescan     bool     `json:"full_rescan"`
	RejectedEvents uint64   `json:"rejected_events"`
}

type options struct {
	MinDepth          int    `json:"min_depth"`
	MaxDepth          *int   `json:"max_depth"`
	MaxOpen           int    `json:"max_open"`
	SameFileSystem    bool   `json:"same_file_system"`
	FollowLinks       bool   `json:"follow_links"`
	CollectMetadata   bool   `json:"collect_metadata"`
	ErrorPolicy       string `json:"error_policy"`
	RootSymlinkPolicy string `json:"root_symlink_policy"`
	SortByFileName    bool   `json:"sort_by_file_name"`
	ContentsFirst     bool   `json:"contents_first"`
}

type entryJSON struct {
	Relative   string  `json:"relative"`
	Depth      int     `json:"depth"`
	IsFile     bool    `json:"is_file"`
	IsDir      bool    `json:"is_dir"`
	IsSymlink  bool    `json:"is_symlink"`
	Bytes      *uint64 `json:"bytes"`
	SkipReason *string `json:"skip_reason"`
}

type scanFileJSON struct {
	Relative           string  `json:"relative"`
	Bytes              uint64  `json:"bytes"`
	ContentHash        *string `json:"content_hash"`
	ContentFingerprint *string `json:"content_fingerprint"`
	BinaryChecked      bool    `json:"binary_checked"`
}

type skipJSON struct {
	Relative string  `json:"relative"`
	Kind     string  `json:"kind"`
	Detail   *string `json:"detail"`
}

type warningJSON struct {
	Relative *string `json:"relative"`
	Message  string  `json:"message"`
}

type sourceJSON struct {
	Kind        string `json:"kind"`
	Location    string `json:"location"`
	ContentHash string `json:"content_hash"`
}

type scanDataJSON struct {
	Files         []scanFileJSON `json:"files"`
	Skipped       []skipJSON     `json:"skipped"`
	Warnings      []warningJSON  `json:"warnings"`
	IgnoreSources []sourceJSON   `json:"ignore_sources"`
	Revision      string         `json:"revision"`
	Descriptor    descriptorJSON `json:"descriptor"`
	Complete      bool           `json:"complete"`
	Termination   *string        `json:"termination"`
	Portable      bool           `json:"portable"`
	Cache         scanCacheJSON  `json:"cache"`
	WatchReason   *string        `json:"watch_reason,omitempty"`
}

type descriptorJSON struct {
	Version uint32 `json:"version"`
	Policy  string `json:"policy"`
}

type scanCacheJSON struct {
	ReusedHashes     uint64 `json:"reused_hashes"`
	ContentReads     uint64 `json:"content_reads"`
	FingerprintReads uint64 `json:"fingerprint_reads"`
}

type response struct {
	Data   any     `json:"data"`
	Timing any     `json:"timing"`
	Error  *string `json:"error"`
}

func main() {
	req, err := decodeRequest()
	if err != nil {
		fail(err.Error())
	}
	run(req)
}

func decodeRequest() (request, error) {
	var req request
	err := json.NewDecoder(os.Stdin).Decode(&req)
	return req, err
}

func run(req request) {
	if handled := runScanOp(req); handled {
		return
	}
	if req.Op != "raw_walk_serial" && req.Op != "raw_walk_sorted" {
		fail("unknown op " + req.Op)
	}
	runWalk(req)
}

func runScanOp(req request) bool {
	switch req.Op {
	case "scan", "scan_compact", "scan_paths", "scan_cached", "scan_incremental", "scan_watch":
		runSessionScan(req)
		return true
	default:
		return false
	}
}

func scanData(report *treestamp.ScanReport) scanDataJSON {
	data := scanMeta(
		report.Revision, report.Descriptor, report.Complete, report.Termination,
		report.Portable, report.Cache,
	)
	for _, file := range report.Files {
		data.Files = append(data.Files, scanFileJSON{
			Relative: file.Relative, Bytes: file.Bytes,
			ContentHash: optional(file.ContentHash), ContentFingerprint: optional(file.ContentFingerprint),
			BinaryChecked: file.BinaryChecked,
		})
	}
	addEvidence(&data, report.Skipped, report.Warnings, report.IgnoreSources)
	return data
}

func compactData(report *treestamp.CompactScanReport) scanDataJSON {
	data := scanMeta(
		report.Revision, report.Descriptor, report.Complete, report.Termination,
		report.Portable, report.Cache,
	)
	for _, file := range report.Files {
		item := scanFileJSON{Relative: file.Relative, Bytes: file.Bytes, ContentHash: optional(file.ContentHash)}
		if file.Content != nil {
			item.ContentFingerprint = optional(file.Content.ContentFingerprint)
			item.BinaryChecked = file.Content.BinaryChecked
		}
		data.Files = append(data.Files, item)
	}
	addEvidence(&data, report.Skipped, report.Warnings, report.IgnoreSources)
	return data
}

func scanMeta(revision string, descriptor treestamp.ScanDescriptor, complete bool, termination treestamp.ScanTermination, portable bool, cache treestamp.ScanCacheStats) scanDataJSON {
	var stopped *string
	if termination != treestamp.TerminationNone {
		value := termination.String()
		stopped = &value
	}
	return scanDataJSON{
		Revision: revision, Descriptor: descriptorJSON(descriptor), Complete: complete,
		Termination: stopped, Portable: portable, Cache: scanCacheJSON{
			ReusedHashes: cache.ReusedHashes, ContentReads: cache.ContentReads, FingerprintReads: cache.FingerprintReads,
		},
		Files: []scanFileJSON{}, Skipped: []skipJSON{}, Warnings: []warningJSON{}, IgnoreSources: []sourceJSON{},
	}
}

func addEvidence(data *scanDataJSON, skipped []treestamp.SkippedEntry, warnings []treestamp.ScanWarning, sources []treestamp.IgnoreSourceEvidence) {
	for _, item := range skipped {
		data.Skipped = append(data.Skipped, skipJSON{Relative: item.Relative, Kind: item.Kind.String(), Detail: optional(item.Detail)})
	}
	for _, item := range warnings {
		data.Warnings = append(data.Warnings, warningJSON{Relative: optional(item.Relative), Message: item.Message})
	}
	for _, item := range sources {
		data.IgnoreSources = append(data.IgnoreSources, sourceJSON{Kind: item.Kind.String(), Location: item.Location, ContentHash: item.ContentHash})
	}
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func runWalk(req request) {
	opts := treestamp.DefaultWalkOptions()
	sorted, contentsFirst := req.Op == "raw_walk_sorted", false
	if len(req.Options) > 0 {
		var parsed options
		if err := json.Unmarshal(req.Options, &parsed); err != nil {
			fail(err.Error())
		}
		applyWalkOptions(&opts, parsed)
		if req.Op == "raw_walk_sorted" {
			parsed.SortByFileName = true
		}
		sorted, contentsFirst = parsed.SortByFileName, parsed.ContentsFirst
	}
	entries, err := walk(req.Root, opts, sorted, contentsFirst)
	if err != nil {
		fail(err.Error())
	}
	write(response{Data: map[string]any{"entries": entries}})
}

func applyWalkOptions(opts *treestamp.WalkOptions, parsed options) {
	opts.MinDepth = parsed.MinDepth
	opts.MaxDepth = parsed.MaxDepth
	if parsed.MaxOpen > 0 {
		opts.MaxOpen = parsed.MaxOpen
	}
	opts.SameFileSystem = parsed.SameFileSystem
	opts.FollowLinks = parsed.FollowLinks
	opts.CollectMetadata = parsed.CollectMetadata
	if parsed.ErrorPolicy == "abort" {
		opts.ErrorPolicy = treestamp.ErrorAbort
	}
	if parsed.RootSymlinkPolicy == "reject" {
		opts.RootSymlinkPolicy = treestamp.RootReject
	}
}

func walk(root string, opts treestamp.WalkOptions, sorted, contentsFirst bool) ([]entryJSON, error) {
	builder := treestamp.NewWalkBuilder(root).Options(opts).ContentsFirst(contentsFirst)
	if sorted {
		builder = builder.SortByFileName()
	}
	walker := builder.Build()
	defer walker.Close()
	var out []entryJSON
	for {
		entry, err := walker.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		item := entryJSON{
			Relative:  filepath.ToSlash(entry.RelativePath()),
			Depth:     entry.Depth(),
			IsFile:    entry.IsFile(),
			IsDir:     entry.IsDir(),
			IsSymlink: entry.IsSymlink(),
			Bytes:     entry.Bytes(),
		}
		if entry.SkipReason() != treestamp.WalkSkipNone {
			reason := entry.SkipReason().String()
			item.SkipReason = &reason
		}
		out = append(out, item)
	}
}

func fail(message string) {
	write(response{Error: &message})
	os.Exit(1)
}

func write(resp response) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(resp)
}
