package platform

import "os"

// StdoutIdentity returns the identity of redirected standard output when it
// is a regular disk file. A console or pipe yields ok=false.
func StdoutIdentity() (Identity, bool) {
	return stdoutIdentity()
}

// PathMatchesIdentity reports whether path refers to the same file as expected.
func PathMatchesIdentity(path string, expected Identity) (bool, error) {
	actual, err := PathIdentity(path)
	if err != nil {
		return false, err
	}
	return actual.Equal(expected), nil
}

func isRegularFile(info os.FileInfo) bool {
	return info != nil && info.Mode().IsRegular()
}
