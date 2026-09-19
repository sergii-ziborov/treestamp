package godirwalk

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Dirent is the godirwalk directory entry.
type Dirent struct {
	name     string
	path     string
	modeType os.FileMode
}

// NewDirent lstats pathname and returns a Dirent.
func NewDirent(osPathname string) (*Dirent, error) {
	info, err := os.Lstat(osPathname)
	if err != nil {
		return nil, err
	}
	return &Dirent{name: filepath.Base(osPathname), path: filepath.Dir(osPathname), modeType: info.Mode().Type()}, nil
}

func (de Dirent) IsDir() bool           { return de.modeType&os.ModeDir != 0 }
func (de Dirent) IsRegular() bool       { return de.modeType&os.ModeType == 0 }
func (de Dirent) IsSymlink() bool       { return de.modeType&os.ModeSymlink != 0 }
func (de Dirent) IsDevice() bool        { return de.modeType&os.ModeDevice != 0 }
func (de Dirent) ModeType() os.FileMode { return de.modeType }
func (de Dirent) Name() string          { return de.name }

func (de Dirent) IsDirOrSymlinkToDir() (bool, error) {
	if de.IsDir() {
		return true, nil
	}
	if !de.IsSymlink() {
		return false, nil
	}
	info, err := os.Stat(filepath.Join(de.path, de.name))
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func direntFrom(path string, d fs.DirEntry) *Dirent {
	if d == nil {
		return &Dirent{name: filepath.Base(path), path: filepath.Dir(path)}
	}
	return &Dirent{name: d.Name(), path: filepath.Dir(path), modeType: d.Type()}
}

// Dirents is a name-sortable slice of Dirent pointers.
type Dirents []*Dirent

func (l Dirents) Len() int           { return len(l) }
func (l Dirents) Less(i, j int) bool { return l[i].name < l[j].name }
func (l Dirents) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
