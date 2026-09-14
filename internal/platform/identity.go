package platform

// Identity is a native file or volume identity. It is not portable across
// machines even when encoded as JSON.
type Identity struct {
	FileSystem uint64
	File       uint64
}

// Info is directory identity used for same-filesystem and symlink-loop checks.
type Info struct {
	FileSystem uint64
	Identity   Identity
}

func (a Identity) Equal(b Identity) bool {
	return a.FileSystem == b.FileSystem && a.File == b.File
}
