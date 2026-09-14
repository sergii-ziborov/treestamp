//go:build windows

package platform

import (
	"os"

	"golang.org/x/sys/windows"
)

// DirectoryInfo returns volume serial and file index for a directory.
func DirectoryInfo(path string) (Info, error) {
	id, err := pathIdentity(path, true)
	if err != nil {
		return Info{}, err
	}
	return Info{FileSystem: id.FileSystem, Identity: id}, nil
}

// FileIdentityFromFile reports identity of an open file or directory handle.
func FileIdentityFromFile(file *os.File) (Identity, error) {
	return handleIdentity(windows.Handle(file.Fd()))
}

// PathIdentity returns identity for a filesystem path.
func PathIdentity(path string) (Identity, error) {
	return pathIdentity(path, false)
}

func pathIdentity(path string, directory bool) (Identity, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Identity{}, err
	}
	flags := uint32(windows.FILE_ATTRIBUTE_NORMAL)
	if directory {
		flags = windows.FILE_FLAG_BACKUP_SEMANTICS
	} else {
		flags = windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	h, err := windows.CreateFile(
		name,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		flags,
		0,
	)
	if err != nil {
		return Identity{}, err
	}
	defer windows.CloseHandle(h)
	return handleIdentity(h)
}

func handleIdentity(h windows.Handle) (Identity, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return Identity{}, err
	}
	return Identity{
		FileSystem: uint64(info.VolumeSerialNumber),
		File:       uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow),
	}, nil
}
