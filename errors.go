package treestamp

import (
	"errors"
	"fmt"
)

// ErrorCode is a stable machine-readable reason.
type ErrorCode string

const (
	// CodeNotImplemented means the requested scan surface is not built yet.
	CodeNotImplemented ErrorCode = "not_implemented"
	// CodeInvalid means the caller passed an unusable option or root.
	CodeInvalid ErrorCode = "invalid"
	// CodeWalk means a filesystem walk operation failed.
	CodeWalk ErrorCode = "walk"
)

// Error is the public library error. Walk-specific fields live on WalkError.
type Error struct {
	Code ErrorCode
	Op   string
	Path string
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	switch {
	case e.Path != "" && e.Err != nil:
		return fmt.Sprintf("treestamp: %s %s: %s: %v", e.Code, e.Op, e.Path, e.Err)
	case e.Path != "":
		return fmt.Sprintf("treestamp: %s %s: %s", e.Code, e.Op, e.Path)
	case e.Err != nil:
		return fmt.Sprintf("treestamp: %s %s: %v", e.Code, e.Op, e.Err)
	default:
		return fmt.Sprintf("treestamp: %s %s", e.Code, e.Op)
	}
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsNotImplemented reports whether err is or wraps CodeNotImplemented.
func IsNotImplemented(err error) bool {
	var typed *Error
	return errors.As(err, &typed) && typed.Code == CodeNotImplemented
}

func notImplemented(op string) error {
	return &Error{Code: CodeNotImplemented, Op: op, Err: errors.New("scanning API is not implemented")}
}
