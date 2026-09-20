package listwalk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/sergii-ziborov/treestamp/internal/dirread"
)

// Stream calls visit for each child without buffering the directory.
func Stream(dir string, depth int, visit func(*Entry) error) error {
	return dirread.Visit(dir, nil, func(name string, typ fs.FileMode, dent fs.DirEntry) error {
		entry := Own(name, child(dir, name), typ, depth, listingInfo(dent))
		if dent != nil {
			entry.Bind(dent)
		}
		return visit(entry)
	})
}

// listingInfo returns metadata already attached to the listing entry.
// It does not Stat: Linux getdents Dent.Info is a later Lstat.
func listingInfo(dent fs.DirEntry) fs.FileInfo {
	if dent == nil {
		return nil
	}
	if _, lazy := dent.(dirread.Dent); lazy {
		return nil
	}
	info, err := dent.Info()
	if err != nil {
		return nil
	}
	return info
}

// Each visits persistable children from one listing. The callback may keep
// the entry; the backing slice stays reachable from that pointer.
func Each(dir string, depth int, visit func(*Entry) error) error {
	dents, err := dirread.OSEntries(dir)
	if err != nil {
		return err
	}
	owned := make([]Entry, len(dents))
	for i, dent := range dents {
		Put(&owned[i], dent.Name(), child(dir, dent.Name()), dent.Type(), depth, listingInfo(dent))
		owned[i].Bind(dent)
		if err := visit(&owned[i]); err != nil {
			return err
		}
	}
	return nil
}

func fill(top *frame, fn fs.WalkDirFunc, cfg Config) error {
	if top.ready {
		return nil
	}
	top.ready = true
	dents, err := dirread.ReadScratch(top.path, nil, cfg.Scratch)
	if err != nil {
		return reportRead(fn, cfg, top.path, err)
	}
	if cfg.Sort {
		sort.Slice(dents, func(i, j int) bool { return dents[i].Name < dents[j].Name })
	}
	if cfg.ContentsFirst {
		dents = filesFirst(dents)
	} else if cfg.DirsFirst {
		dents = dirsFirst(dents)
	}
	top.owned = hold(top, dents)
	return nil
}

func hold(top *frame, dents []dirread.Record) []Entry {
	owned := make([]Entry, len(dents))
	for i, rec := range dents {
		Put(&owned[i], rec.Name, child(top.path, rec.Name), rec.Type, top.depth+1, rec.Info)
	}
	return owned
}

func finish(root string, fn fs.WalkDirFunc, cfg Config, top *frame, skip *string, frames *[]frame) error {
	top.exhaust()
	if *skip == top.path {
		*skip = ""
	}
	if cfg.After != nil {
		entry := Own(filepath.Base(top.path), top.path, os.ModeDir, top.depth, nil)
		err := Deliver(cfg.After, entry, cfg.ToSlash)
		if err != nil && !errors.Is(err, fs.SkipDir) && !skipThis(cfg, err) {
			if errors.Is(err, fs.SkipAll) {
				return errStop
			}
			return err
		}
	}
	*frames = (*frames)[:len(*frames)-1]
	return nil
}

func visit(fn fs.WalkDirFunc, cfg Config, top *frame, rec dirread.Record, frames *[]frame, skip *string) error {
	if *skip == top.path && rec.Type.IsRegular() {
		return nil
	}
	path := child(top.path, rec.Name)
	entry := top.live(rec.Name, path, rec.Type, rec.Info)
	return control(cfg, Deliver(fn, entry, cfg.ToSlash), entry, top, frames, skip)
}

func (f *frame) live(name, path string, typ fs.FileMode, info fs.FileInfo) *Entry {
	if len(f.cur) == cap(f.cur) {
		if cap(f.cur) > 0 {
			f.chunks = append(f.chunks, f.cur)
		}
		n := 32
		if cap(f.cur) >= 32 {
			n = 64
		}
		f.cur = make([]Entry, 0, n)
	}
	f.cur = f.cur[:len(f.cur)+1]
	e := &f.cur[len(f.cur)-1]
	Put(e, name, path, typ, f.depth+1, info)
	return e
}

func control(cfg Config, cbErr error, entry *Entry, top *frame, frames *[]frame, skip *string) error {
	switch {
	case cbErr == nil:
		return descend(cfg, entry, frames)
	case skipThis(cfg, cbErr):
		return nil
	case errors.Is(cbErr, fs.SkipDir):
		if !entry.typ.IsDir() {
			top.exhaust()
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

func descend(cfg Config, entry *Entry, frames *[]frame) error {
	if entry.typ&os.ModeSymlink != 0 {
		if cfg.Follow {
			return follow(cfg, entry, frames)
		}
		return nil
	}
	if entry.typ.IsDir() {
		*frames = append(*frames, frame{path: entry.path, depth: entry.depth})
	}
	return nil
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

func reportRead(fn fs.WalkDirFunc, cfg Config, path string, err error) error {
	cbErr := fn(Show(path, cfg.ToSlash), nil, err)
	if cbErr == nil || errors.Is(cbErr, fs.SkipDir) || skipThis(cfg, cbErr) {
		return nil
	}
	if errors.Is(cbErr, fs.SkipAll) {
		return errStop
	}
	return cbErr
}

func filesFirst(dents []dirread.Record) []dirread.Record {
	var files, dirs, other []dirread.Record
	for _, dent := range dents {
		switch {
		case dent.Type.IsRegular():
			files = append(files, dent)
		case dent.Type.IsDir():
			dirs = append(dirs, dent)
		default:
			other = append(other, dent)
		}
	}
	return append(append(files, other...), dirs...)
}

func dirsFirst(dents []dirread.Record) []dirread.Record {
	var files, dirs, other []dirread.Record
	for _, dent := range dents {
		switch {
		case dent.Type.IsDir():
			dirs = append(dirs, dent)
		case dent.Type.IsRegular():
			files = append(files, dent)
		default:
			other = append(other, dent)
		}
	}
	return append(append(dirs, other...), files...)
}
