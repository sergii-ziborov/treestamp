package walkcfg

import (
	"io/fs"
	"os"
	"sync"

	"github.com/sergii-ziborov/treestamp/internal/platform"
	"github.com/sergii-ziborov/treestamp/internal/walk"
)

const identityShards = 8

// EntryFilter records native file identity. Same path, same object, and
// same bytes are different questions; this answers the second.
type EntryFilter struct {
	ents [identityShards]identityShard
}

type identityShard struct {
	mu   sync.Mutex
	keys map[platform.Identity]struct{}
}

// NewEntryFilter returns an empty identity filter.
func NewEntryFilter() *EntryFilter {
	return &EntryFilter{}
}

// Entry reports whether path was seen. An identity error is not a
// duplicate: the caller still sees the entry.
func (f *EntryFilter) Entry(path string, d fs.DirEntry) bool {
	seen, ok := f.Observe(path, d)
	return ok && seen
}

// Observe records identity when it can be proven. ok is false when the
// platform cannot identify the object; then seen is always false.
func (f *EntryFilter) Observe(path string, d fs.DirEntry) (seen, ok bool) {
	id, ok, err := identityOf(path, d)
	if err != nil || !ok {
		return false, false
	}
	return f.seen(id), true
}

func (f *EntryFilter) seen(id platform.Identity) bool {
	shard := &f.ents[id.File%identityShards]
	shard.mu.Lock()
	if shard.keys == nil {
		shard.keys = make(map[platform.Identity]struct{})
	}
	_, dup := shard.keys[id]
	if !dup {
		shard.keys[id] = struct{}{}
	}
	shard.mu.Unlock()
	return dup
}

func identityOf(path string, d fs.DirEntry) (platform.Identity, bool, error) {
	if d != nil && d.Type()&os.ModeSymlink == 0 {
		if info, err := d.Info(); err == nil {
			if id, ok := platform.IdentityFromInfo(info); ok {
				return id, true, nil
			}
		}
	}
	info, err := walk.StatDirEntry(path, d)
	if err != nil {
		return platform.Identity{}, false, err
	}
	if id, ok := platform.IdentityFromInfo(info); ok {
		return id, true, nil
	}
	id, err := platform.PathIdentity(path)
	if err != nil {
		return platform.Identity{}, false, err
	}
	return id, true, nil
}
