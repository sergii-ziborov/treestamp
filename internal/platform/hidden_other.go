//go:build !unix && !windows

package platform

import "os"

func nativeHidden(_ os.FileInfo) bool {
	return false
}
