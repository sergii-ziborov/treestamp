package treestamp

import "context"

// ScanReport is a placeholder for the future full manifest.
// Scan always returns Err not implemented; do not serialize this type.
type ScanReport struct {
	_ [0]func()
}

// CompactScanReport is a placeholder for the future compact manifest.
type CompactScanReport struct {
	_ [0]func()
}

// Scanner is a placeholder for the reusable configured scanner.
type Scanner struct {
	root string
}

// Scan is the intended one-shot full report API. It is not implemented.
func Scan(_ context.Context, root string) (*ScanReport, error) {
	return nil, notImplemented("Scan")
}

// ScanCompact is the intended compact report API. It is not implemented.
// A later implementation must not build a full report first.
func ScanCompact(_ context.Context, root string) (*CompactScanReport, error) {
	return nil, notImplemented("ScanCompact")
}

// ScanPaths is the intended path-only API. It is not implemented.
// A later implementation must not call Scan and strip fields: that would
// apply max_file_bytes and hashing that the oracle path API does not apply.
func ScanPaths(_ context.Context, root string) ([]string, error) {
	return nil, notImplemented("ScanPaths")
}

// NewScanner is the intended reusable constructor. It is not implemented.
func NewScanner(root string, _ ...ScannerOption) (*Scanner, error) {
	if root == "" {
		return nil, &Error{Code: CodeInvalid, Op: "NewScanner", Err: errEmptyRoot}
	}
	return nil, notImplemented("NewScanner")
}

var errEmptyRoot = errString("root is empty")

type errString string

func (e errString) Error() string { return string(e) }
