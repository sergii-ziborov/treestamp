package walk

import (
	"fmt"
	"io"
	"io/fs"
)

// WalkOperation names the filesystem step that failed.
type WalkOperation int

const (
	OpCanonicalize WalkOperation = iota
	OpReadDirectory
	OpReadEntry
	OpReadMetadata
	OpScheduleWorker
)

func (o WalkOperation) String() string {
	switch o {
	case OpCanonicalize:
		return "canonicalize"
	case OpReadDirectory:
		return "read directory"
	case OpReadEntry:
		return "read entry"
	case OpReadMetadata:
		return "read metadata"
	case OpScheduleWorker:
		return "schedule worker"
	default:
		return "walk"
	}
}

// WalkError is a typed traversal failure.
type WalkError struct {
	Path      string
	Depth     int
	Operation WalkOperation
	Err       error
}

func (e *WalkError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s at depth %d for %s: %v", e.Operation, e.Depth, e.Path, e.Err)
}

func (e *WalkError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func walkErr(path string, depth int, op WalkOperation, err error) *WalkError {
	return &WalkError{Path: path, Depth: depth, Operation: op, Err: err}
}

func isEOF(err error) bool {
	return err == io.EOF || err == fs.ErrClosed
}
