//go:build !windows

package store

import "os"

func replaceFile(tmp, dest string) error {
	return os.Rename(tmp, dest)
}
