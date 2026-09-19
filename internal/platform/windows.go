//go:build windows

package platform

import (
	"os"
	"syscall"

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
	flags := uint32(windows.FILE_FLAG_BACKUP_SEMANTICS)
	_ = directory
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

func identityFromInfo(_ os.FileInfo) (Identity, bool) {
	// FindFirstFile / Lstat expose Win32FileAttributeData, not a file index.
	return Identity{}, false
}

func nativeHidden(info os.FileInfo) bool {
	if info == nil {
		return false
	}
	switch d := info.Sys().(type) {
	case *windows.Win32FileAttributeData:
		return d.FileAttributes&windows.FILE_ATTRIBUTE_HIDDEN != 0
	case *syscall.Win32FileAttributeData:
		return d.FileAttributes&windows.FILE_ATTRIBUTE_HIDDEN != 0
	case windows.Win32FileAttributeData:
		return d.FileAttributes&windows.FILE_ATTRIBUTE_HIDDEN != 0
	case syscall.Win32FileAttributeData:
		return d.FileAttributes&windows.FILE_ATTRIBUTE_HIDDEN != 0
	}
	type attr interface {
		FileAttributes() uint32
	}
	if a, ok := info.Sys().(attr); ok {
		return a.FileAttributes()&windows.FILE_ATTRIBUTE_HIDDEN != 0
	}
	return false
}

func stdoutIdentity() (Identity, bool) {
	h := windows.Handle(os.Stdout.Fd())
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return Identity{}, false
	}
	if info.VolumeSerialNumber == 0 && info.FileIndexHigh == 0 && info.FileIndexLow == 0 {
		return Identity{}, false
	}
	stat, err := os.Stdout.Stat()
	if err != nil || !isRegularFile(stat) {
		return Identity{}, false
	}
	return Identity{
		FileSystem: uint64(info.VolumeSerialNumber),
		File:       uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow),
	}, true
}
