//go:build windows

package platform

import (
	"os"

	"golang.org/x/sys/windows"
)

func stdoutIdentity() (Identity, bool) {
	h := windows.Handle(os.Stdout.Fd())
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return Identity{}, false
	}
	// A console or pipe typically fails the information call or is not a disk file.
	// FileIndex 0 with no volume is treated as absent.
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
