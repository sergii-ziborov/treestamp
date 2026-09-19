// Package dirread lists directory names and types without extra metadata.
package dirread

import (
	"errors"
	"io/fs"
	"os"
)

// ErrTruncatedRecord is returned when a getdents buffer ends mid-record.
var ErrTruncatedRecord = errors.New("truncated directory record")

// Record is a name and file mode type from a directory read.
type Record struct {
	Name string
	Type fs.FileMode
	Info fs.FileInfo
}

// UnknownType means the filesystem did not report a dirent type.
const UnknownType fs.FileMode = ^fs.FileMode(0)

// Dent is a compact directory entry from a name and type.
type Dent struct {
	dir  string
	name string
	typ  fs.FileMode
}

func (d Dent) Name() string               { return d.name }
func (d Dent) IsDir() bool                { return d.typ.IsDir() }
func (d Dent) IsRegular() bool            { return d.typ.IsRegular() }
func (d Dent) IsSymlink() bool            { return d.typ&fs.ModeSymlink != 0 }
func (d Dent) Type() fs.FileMode          { return d.typ }
func (d Dent) ModeType() fs.FileMode      { return d.typ.Type() }
func (d Dent) Info() (fs.FileInfo, error) { return os.Lstat(joinName(d.dir, d.name)) }

// Reported is true when mode came from the listing. Zero is a regular file.
func Reported(mode fs.FileMode) bool { return mode != UnknownType }

// Kind classifies a listing mode. UnknownType is neither a file nor a dir.
func Kind(mode fs.FileMode) (isDir, isFile, symlink bool) {
	if !Reported(mode) {
		return false, false, false
	}
	symlink = mode&fs.ModeSymlink != 0
	return !symlink && mode.IsDir(), !symlink && mode.IsRegular(), symlink
}

// ModeOf returns the listing type. Type()==0 is regular and does not call Info.
func ModeOf(entry fs.DirEntry) (fs.FileMode, error) {
	mode := entry.Type()
	if Reported(mode) {
		return mode, nil
	}
	info, err := entry.Info()
	if err != nil {
		return 0, err
	}
	return info.Mode().Type(), nil
}

func joinName(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + string(os.PathSeparator) + name
}
