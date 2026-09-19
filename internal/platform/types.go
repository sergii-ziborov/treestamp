package platform

import (
	"os"
	"path/filepath"
	"strings"
)

// Identity is a native file or volume identity. It is not portable across
// machines even when encoded as JSON.
type Identity struct {
	FileSystem uint64 `json:"file_system"`
	File       uint64 `json:"file"`
}

// Info is directory identity used for same-filesystem and symlink-loop checks.
type Info struct {
	FileSystem uint64
	Identity   Identity
}

func (a Identity) Equal(b Identity) bool {
	return a.FileSystem == b.FileSystem && a.File == b.File
}

// HiddenName reports whether the final path element starts with '.'.
func HiddenName(path string) bool {
	base := filepath.Base(path)
	return strings.HasPrefix(base, ".")
}

// Hidden reports name-based hiding plus native hidden attributes when known.
func Hidden(path string, native *bool) bool {
	if HiddenName(path) {
		return true
	}
	if native != nil {
		return *native
	}
	return false
}

// HiddenFromInfo uses platform metadata. On Unix only the name is used.
func HiddenFromInfo(path string, info os.FileInfo) bool {
	if HiddenName(path) {
		return true
	}
	return nativeHidden(info)
}

// StdoutIdentity returns the identity of redirected standard output when it
// is a regular disk file. A console or pipe yields ok=false.
func StdoutIdentity() (Identity, bool) {
	return stdoutIdentity()
}

// PathMatchesIdentity reports whether path refers to the same file as expected.
func PathMatchesIdentity(path string, expected Identity) (bool, error) {
	actual, err := PathIdentity(path)
	if err != nil {
		return false, err
	}
	return actual.Equal(expected), nil
}

func isRegularFile(info os.FileInfo) bool {
	return info != nil && info.Mode().IsRegular()
}

// IdentityFromInfo extracts native identity from FileInfo.Sys when the
// platform left device and inode (or equivalent) on that value.
func IdentityFromInfo(info os.FileInfo) (Identity, bool) {
	return identityFromInfo(info)
}
