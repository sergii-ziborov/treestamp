package listwalk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/sergii-ziborov/treestamp/internal/dirread"
)

func fill(top *frame, fn fs.WalkDirFunc, cfg Config) error {
	if top.ready {
		return nil
	}
	top.ready = true
	dents, err := dirread.OSEntries(top.path)
	if err != nil {
		cbErr := fn(Show(top.path, cfg.ToSlash), nil, err)
		if cbErr == nil || errors.Is(cbErr, fs.SkipDir) {
			return nil
		}
		if errors.Is(cbErr, fs.SkipAll) {
			return errStop
		}
		return cbErr
	}
	if cfg.ContentsFirst {
		dents = filesFirst(dents)
	}
	top.dents = dents
	return nil
}

func finish(root string, fn fs.WalkDirFunc, cfg Config, top *frame, skip *string, frames *[]frame) error {
	if *skip == top.path {
		*skip = ""
	}
	if cfg.After != nil && top.path != root {
		entry := Acquire(filepath.Base(top.path), top.path, os.ModeDir, top.depth, nil)
		err := Call(cfg.After, entry, cfg.ToSlash)
		Release(entry)
		if err != nil && !errors.Is(err, fs.SkipDir) {
			if errors.Is(err, fs.SkipAll) {
				return errStop
			}
			return err
		}
	}
	*frames = (*frames)[:len(*frames)-1]
	return nil
}

func visit(fn fs.WalkDirFunc, cfg Config, top *frame, dent os.DirEntry, frames *[]frame, skip *string) error {
	if *skip == top.path && dent.Type().IsRegular() {
		return nil
	}
	path := child(top.path, dent.Name())
	entry := Acquire(dent.Name(), path, dent.Type(), top.depth+1, nil)
	entry.Bind(dent)
	err := control(cfg, Call(fn, entry, cfg.ToSlash), entry, top, frames, skip)
	Release(entry)
	return err
}

func control(cfg Config, cbErr error, entry *Entry, top *frame, frames *[]frame, skip *string) error {
	switch {
	case cbErr == nil:
		if entry.typ.IsDir() {
			*frames = append(*frames, frame{path: entry.path, depth: entry.depth})
		}
		return nil
	case errors.Is(cbErr, fs.SkipDir):
		if !entry.typ.IsDir() {
			top.index = len(top.dents)
		}
		return nil
	case errors.Is(cbErr, fs.SkipAll):
		return errStop
	case cfg.SkipFiles != nil && errors.Is(cbErr, cfg.SkipFiles):
		*skip = top.path
		return nil
	case cfg.TraverseLink != nil && errors.Is(cbErr, cfg.TraverseLink) && entry.typ&os.ModeSymlink != 0:
		return follow(cfg, entry, frames)
	default:
		return cbErr
	}
}

func follow(cfg Config, entry *Entry, frames *[]frame) error {
	if cfg.OnLink == nil {
		return cfg.TraverseLink
	}
	ancestors := make([]string, len(*frames))
	for i, fr := range *frames {
		ancestors[i] = fr.path
	}
	ok, err := cfg.OnLink(entry.path, entry.name, entry.depth, ancestors)
	if err != nil || !ok {
		return err
	}
	*frames = append(*frames, frame{path: entry.path, depth: entry.depth})
	return nil
}

func child(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + string(os.PathSeparator) + name
}

func filesFirst(dents []os.DirEntry) []os.DirEntry {
	var files, dirs, other []os.DirEntry
	for _, dent := range dents {
		switch {
		case dent.Type().IsRegular():
			files = append(files, dent)
		case dent.IsDir():
			dirs = append(dirs, dent)
		default:
			other = append(other, dent)
		}
	}
	return append(append(files, other...), dirs...)
}
