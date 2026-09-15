//go:build unix

package platform

import (
	"os"

	"golang.org/x/sys/unix"
)

// DirectoryInfo returns device and inode for a directory.
func DirectoryInfo(path string) (Info, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return Info{}, err
	}
	id := Identity{FileSystem: uint64(st.Dev), File: uint64(st.Ino)}
	return Info{FileSystem: id.FileSystem, Identity: id}, nil
}

// FileIdentityFromFile reports identity of an open regular file.
func FileIdentityFromFile(file *os.File) (Identity, error) {
	var st unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &st); err != nil {
		return Identity{}, err
	}
	return Identity{FileSystem: uint64(st.Dev), File: uint64(st.Ino)}, nil
}

// PathIdentity returns identity for a filesystem path.
func PathIdentity(path string) (Identity, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return Identity{}, err
	}
	return Identity{FileSystem: uint64(st.Dev), File: uint64(st.Ino)}, nil
}

func nativeHidden(_ os.FileInfo) bool {
	return false
}

func stdoutIdentity() (Identity, bool) {
	info, err := os.Stdout.Stat()
	if err != nil || !isRegularFile(info) {
		return Identity{}, false
	}
	id, err := FileIdentityFromFile(os.Stdout)
	if err != nil {
		return Identity{}, false
	}
	return id, true
}
