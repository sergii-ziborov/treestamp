// Command treestamp-driver speaks the fixture protocol for the Go walker.
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
	Options json.RawMessage `json:"options"`
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

type response struct {
	Data   any     `json:"data"`
	Timing any     `json:"timing"`
	Error  *string `json:"error"`
}

func main() {
	var req request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		fail(err.Error())
	}
	switch req.Op {
	case "raw_walk_serial", "raw_walk_sorted":
	case "scan", "scan_compact", "scan_paths":
		msg := "scanning API is not implemented"
		write(response{Error: &msg})
		os.Exit(2)
	default:
		fail("unknown op " + req.Op)
	}
	opts := treestamp.DefaultWalkOptions()
	if len(req.Options) > 0 {
		var parsed options
		if err := json.Unmarshal(req.Options, &parsed); err != nil {
			fail(err.Error())
		}
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
		if req.Op == "raw_walk_sorted" {
			parsed.SortByFileName = true
		}
		entries, err := walk(req.Root, opts, parsed.SortByFileName, parsed.ContentsFirst)
		if err != nil {
			fail(err.Error())
		}
		write(response{Data: map[string]any{"entries": entries}})
		return
	}
	entries, err := walk(req.Root, opts, req.Op == "raw_walk_sorted", false)
	if err != nil {
		fail(err.Error())
	}
	write(response{Data: map[string]any{"entries": entries}})
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
		if entry.SkipReason() != treestamp.SkipNone {
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
