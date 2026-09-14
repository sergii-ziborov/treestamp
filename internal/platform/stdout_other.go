//go:build !unix && !windows

package platform

func stdoutIdentity() (Identity, bool) {
	return Identity{}, false
}
