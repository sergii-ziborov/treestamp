//go:build windows

package platform

import (
	"os"

	"golang.org/x/sys/windows"
)

func nativeHidden(info os.FileInfo) bool {
	type attr interface {
		FileAttributes() uint32
	}
	if a, ok := info.Sys().(attr); ok {
		return a.FileAttributes()&windows.FILE_ATTRIBUTE_HIDDEN != 0
	}
	if d, ok := info.Sys().(*windows.Win32FileAttributeData); ok {
		return d.FileAttributes&windows.FILE_ATTRIBUTE_HIDDEN != 0
	}
	return false
}
