//go:build !unix && !windows

package platform

import (
	"fmt"
	"os"
)

// DirectoryInfo is unsupported on this platform.
func DirectoryInfo(path string) (Info, error) {
	return Info{}, fmt.Errorf("filesystem identity is unsupported on this platform: %s", path)
}

// FileIdentityFromFile is unsupported on this platform.
func FileIdentityFromFile(file *os.File) (Identity, error) {
	return Identity{}, fmt.Errorf("filesystem identity is unsupported")
}

// PathIdentity is unsupported on this platform.
func PathIdentity(path string) (Identity, error) {
	return Identity{}, fmt.Errorf("filesystem identity is unsupported: %s", path)
}

func nativeHidden(_ os.FileInfo) bool {
	return false
}

func stdoutIdentity() (Identity, bool) {
	return Identity{}, false
}
