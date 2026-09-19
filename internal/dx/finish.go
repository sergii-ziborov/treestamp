package dx

import (
	"context"
	"errors"
	"io/fs"
	"strconv"
	"strings"
)

var (
	ErrPartial = errors.New("incomplete selected work")
	ErrStop    = errors.New("stopped")
)

const (
	TermNone = iota
	TermMaxEntries
	TermMaxTotalBytes
	TermTimeout
	TermCancelled
)

type Outcome struct {
	Complete bool
	Term     int
	Unread   bool
}

func Check(ctx context.Context, o Outcome) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch o.Term {
	case TermCancelled:
		return context.Canceled
	case TermTimeout:
		return context.DeadlineExceeded
	}
	if o.Unread || !o.Complete {
		return ErrPartial
	}
	return nil
}

func Finish(ctx context.Context, o Outcome, stopped bool) error {
	if stopped {
		return nil
	}
	return Check(ctx, o)
}

func Classify(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrPartial):
		return "partial"
	case errors.Is(err, fs.ErrPermission):
		return "permission"
	case errors.Is(err, fs.ErrNotExist):
		return "unavailable"
	default:
		return "walk"
	}
}

func Clone(ss []string) []string { return append([]string(nil), ss...) }

func Describe(ignore []string, skipHidden, standard, hash, binary, failFast bool, maxBytes uint64) string {
	callback := "none"
	if failFast {
		callback = "fail_fast"
	}
	return strings.Join([]string{
		"profile=repository",
		"ignore_files=" + strings.Join(ignore, ","),
		"skip_hidden=" + strconv.FormatBool(skipHidden),
		"standard_skips=" + strconv.FormatBool(standard),
		"hash=" + strconv.FormatBool(hash),
		"detect_binary=" + strconv.FormatBool(binary),
		"max_file_bytes=" + strconv.FormatUint(maxBytes, 10),
		"callback=" + callback,
		"order=relative",
	}, " ")
}
