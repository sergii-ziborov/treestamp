package report

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sergii-ziborov/treestamp/internal/hashx"
	pathx "github.com/sergii-ziborov/treestamp/internal/path"
)

const (
	EvidenceFileVersion = 0
	EvidenceSHA256      = 1
)

type File struct {
	Absolute, Relative, ContentHash string
	Bytes                           uint64
	ModifiedNS                      *uint64
}

type Error struct {
	Relative        string
	Reason          string
	Bytes, MaxBytes uint64
	Err             error
}

func (e *Error) Error() string {
	if e == nil {
		return "snapshot read error"
	}
	rel, extra := e.Relative, ""
	if e.Err != nil {
		extra = ": " + e.Err.Error()
	}
	switch e.Reason {
	case "unknown":
		return "file is not present in the scan snapshot: " + rel
	case "stale":
		return "scan snapshot is stale: " + rel
	case "limit":
		return fmt.Sprintf("scan snapshot file exceeds content limit: %s (%d > %d)", rel, e.Bytes, e.MaxBytes)
	case "invalid":
		if rel == "" {
			rel = "<root>"
		}
		return "invalid scan report entry " + rel + extra
	case "io":
		return "could not read scan snapshot file " + rel + extra
	default:
		if extra != "" {
			return rel + extra
		}
		return rel
	}
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func invalid(rel, msg string) error {
	return &Error{Relative: rel, Reason: "invalid", Err: errors.New(msg)}
}

func stale(rel string) error { return &Error{Relative: rel, Reason: "stale"} }

func Validate(root string, files []File) error {
	if root == "" || !filepath.IsAbs(root) {
		reason := "root is empty"
		if root != "" {
			reason = "root is not absolute"
		}
		return invalid("", reason)
	}
	var previous string
	for i, file := range files {
		if i > 0 && previous >= file.Relative {
			return invalid(file.Relative, "files are not strictly sorted")
		}
		previous = file.Relative
		if file.Absolute == "" || !pathx.UnderRoot(root, file.Absolute) {
			return invalid(file.Relative, "absolute path escapes root")
		}
		rel, err := filepath.Rel(root, file.Absolute)
		if err != nil || slashRel(rel) != file.Relative {
			return invalid(file.Relative, "relative and absolute paths disagree")
		}
		if file.ContentHash == "" && file.ModifiedNS == nil {
			return invalid(file.Relative, "entry has no reusable snapshot evidence")
		}
	}
	return nil
}

func Lookup(files []File, relative string) (File, bool) {
	lo, hi := 0, len(files)
	for lo < hi {
		mid := (lo + hi) / 2
		switch {
		case files[mid].Relative < relative:
			lo = mid + 1
		case files[mid].Relative > relative:
			hi = mid
		default:
			return files[mid], true
		}
	}
	for _, file := range files {
		if file.Relative == relative {
			return file, true
		}
	}
	return File{}, false
}

func ReadBounded(root string, files []File, relative string, maxBytes uint64) ([]byte, int, error) {
	file, ok := Lookup(files, relative)
	if !ok {
		return nil, 0, &Error{Relative: relative, Reason: "unknown"}
	}
	if file.Bytes > maxBytes {
		return nil, 0, &Error{Relative: relative, Reason: "limit", Bytes: file.Bytes, MaxBytes: maxBytes}
	}
	return readBytes(root, file)
}

func readBytes(root string, snapshot File) ([]byte, int, error) {
	relative := snapshot.Relative
	info, err := os.Lstat(snapshot.Absolute)
	if err != nil {
		return nil, 0, mapIO(relative, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, 0, stale(relative)
	}
	abs, err := filepath.Abs(snapshot.Absolute)
	if err != nil {
		return nil, 0, mapIO(relative, err)
	}
	if !pathx.UnderRoot(root, abs) {
		return nil, 0, stale(relative)
	}
	return openAndHash(snapshot)
}

func openAndHash(snapshot File) ([]byte, int, error) {
	relative := snapshot.Relative
	f, err := os.Open(snapshot.Absolute)
	if err != nil {
		return nil, 0, mapIO(relative, err)
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return nil, 0, mapIO(relative, err)
	}
	if !before.Mode().IsRegular() || uint64(before.Size()) != snapshot.Bytes {
		return nil, 0, stale(relative)
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(snapshot.Bytes)+1))
	if err != nil {
		return nil, 0, mapIO(relative, err)
	}
	if uint64(len(data)) != snapshot.Bytes {
		return nil, 0, stale(relative)
	}
	after, err := f.Stat()
	if err != nil {
		return nil, 0, mapIO(relative, err)
	}
	if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return nil, 0, stale(relative)
	}
	if snapshot.ContentHash != "" {
		if hashx.SHA256Prefix(data) != snapshot.ContentHash {
			return nil, 0, stale(relative)
		}
		return data, EvidenceSHA256, nil
	}
	if snapshot.ModifiedNS == nil {
		return nil, 0, stale(relative)
	}
	return data, EvidenceFileVersion, nil
}

func mapIO(relative string, err error) error {
	if os.IsNotExist(err) {
		return stale(relative)
	}
	return &Error{Relative: relative, Reason: "io", Err: err}
}

func slashRel(rel string) string {
	if rel == "." {
		return ""
	}
	return pathx.Slash(rel)
}
