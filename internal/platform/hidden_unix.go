//go:build unix

package platform

import "os"

func nativeHidden(_ os.FileInfo) bool {
	return false
}
