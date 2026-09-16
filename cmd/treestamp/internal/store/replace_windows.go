//go:build windows

package store

import (
	"os"

	"golang.org/x/sys/windows"
)

func replaceFile(tmp, dest string) error {
	from, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(dest)
	if err != nil {
		return err
	}
	err = windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
	if err == nil {
		return nil
	}
	if _, statErr := os.Stat(dest); os.IsNotExist(statErr) {
		return os.Rename(tmp, dest)
	}
	return err
}
