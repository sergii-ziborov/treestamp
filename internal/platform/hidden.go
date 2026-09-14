package platform

import (
	"os"
	"path/filepath"
	"strings"
)

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
