// Package dirread lists directory names and types without extra metadata.
package dirread

import (
	"io/fs"
	"os"
)

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

func joinName(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + string(os.PathSeparator) + name
}
