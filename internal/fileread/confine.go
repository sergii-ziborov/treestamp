package fileread

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	pathx "github.com/sergii-ziborov/treestamp/internal/path"
	"github.com/sergii-ziborov/treestamp/internal/platform"
	"github.com/sergii-ziborov/treestamp/internal/walk"
)

var (
	ErrEscape  = errors.New("path escapes scan root")
	ErrSymlink = errors.New("refusing to read a symbolic link")
)

func Confine(root, path string) (string, error) {
	return ConfineAt(root, path, false)
}

func ConfineAt(root, path string, follow bool) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rootInfo, err := os.Lstat(rootAbs)
	if err != nil {
		return "", err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", ErrEscape
	}
	if !pathx.UnderRoot(rootAbs, abs) {
		return "", ErrEscape
	}
	if follow {
		return confineFollowed(rootAbs, abs)
	}
	return confineLiteral(rootAbs, abs)
}

func confineFollowed(rootAbs, abs string) (string, error) {
	resolved, err := pathx.Resolve(abs)
	if err != nil {
		return "", err
	}
	if !pathx.UnderRoot(rootAbs, resolved) {
		return "", ErrEscape
	}
	return resolved, nil
}

func confineLiteral(rootAbs, abs string) (string, error) {
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil {
		return "", err
	}
	cur := rootAbs
	if rel == "." {
		return abs, nil
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if cur == abs {
				return "", ErrSymlink
			}
			return "", ErrEscape
		}
	}
	return abs, nil
}

func FromInfo(path string, info os.FileInfo) walk.FileVersion {
	ver := walk.FileVersion{}
	if info != nil && !info.ModTime().IsZero() && info.ModTime().After(time.Unix(0, 0)) {
		ns := uint64(info.ModTime().UnixNano())
		ver.ModifiedNS = &ns
	}
	if id, err := platform.PathIdentity(path); err == nil {
		ver.Identity = &id
	}
	return ver
}

func FromFile(f *os.File) (walk.FileVersion, os.FileInfo, error) {
	info, err := f.Stat()
	if err != nil {
		return walk.FileVersion{}, nil, err
	}
	ver := walk.FileVersion{}
	if !info.ModTime().IsZero() && info.ModTime().After(time.Unix(0, 0)) {
		ns := uint64(info.ModTime().UnixNano())
		ver.ModifiedNS = &ns
	}
	if id, err := platform.FileIdentityFromFile(f); err == nil {
		ver.Identity = &id
	}
	return ver, info, nil
}

func SameObject(opened, discovered walk.FileVersion, size, want uint64) bool {
	if size != want {
		return false
	}
	if discovered.ModifiedNS != nil && (opened.ModifiedNS == nil || *opened.ModifiedNS != *discovered.ModifiedNS) {
		return false
	}
	if discovered.Identity != nil && opened.Identity != nil && !discovered.Identity.Equal(*opened.Identity) {
		return false
	}
	return true
}
