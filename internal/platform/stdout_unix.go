//go:build unix

package platform

import "os"

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
