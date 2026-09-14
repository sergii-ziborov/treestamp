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
	info, err := file.Stat()
	if err != nil {
		return Identity{}, err
	}
	return identityFromFileInfo(info)
}

func identityFromFileInfo(info os.FileInfo) (Identity, error) {
	st, ok := info.Sys().(*unix.Stat_t)
	if !ok {
		var raw unix.Stat_t
		if err := unix.Stat(info.Name(), &raw); err != nil {
			return Identity{}, err
		}
		return Identity{FileSystem: uint64(raw.Dev), File: uint64(raw.Ino)}, nil
	}
	return Identity{FileSystem: uint64(st.Dev), File: uint64(st.Ino)}, nil
}

// PathIdentity returns identity for a filesystem path.
func PathIdentity(path string) (Identity, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Identity{}, err
	}
	return identityFromFileInfo(info)
}
