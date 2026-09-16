// Package merkle holds a persistent keyed snapshot tree.
// TreeRevision is versioned separately from the legacy flat scan digest.
package merkle

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

const Version = 2

const prefix = "tree2:"

type Record struct {
	Path, Hash string
	Size       uint64
}

type Coverage struct {
	RootIdentity string
	Policy       string
	Epoch        uint64
	Cursor       string
	Complete     bool
	Overflow     bool
}

func (c Coverage) RequiresRecheck() bool {
	return !c.Complete || c.Overflow
}

type node struct {
	key        string
	pri        uint64
	rec        Record
	left, right *node
	count      int
	sum        [32]byte
}

// Tree is an immutable persistent treap of file records.
// Shape is determined by keys, so rebuild and incremental apply match.
type Tree struct {
	root *node
	n    int
}

func New() *Tree { return &Tree{} }

func Build(recs []Record) *Tree {
	t := New()
	for _, rec := range recs {
		t = t.Upsert(rec)
	}
	return t
}

func (t *Tree) Len() int {
	if t == nil {
		return 0
	}
	return t.n
}

func (t *Tree) Revision() string {
	if t == nil || t.root == nil {
		return prefix + hex.EncodeToString(emptySum[:])
	}
	return prefix + hex.EncodeToString(t.root.sum[:])
}

func (t *Tree) Upsert(rec Record) *Tree {
	cur := t
	if cur == nil {
		cur = New()
	}
	next := *cur
	next.root = upsert(cur.root, rec)
	next.n = countOf(next.root)
	return &next
}

func (t *Tree) Delete(path string) *Tree {
	if t == nil || t.root == nil {
		return New()
	}
	next := *t
	next.root = remove(t.root, path)
	next.n = countOf(next.root)
	return &next
}

// Diff returns upserts and deletes that turn prev into cur. Unchanged keys are omitted.
func Diff(prev, cur []Record) (upserts, deletes []Record) {
	byPrev := make(map[string]Record, len(prev))
	for _, rec := range prev {
		byPrev[rec.Path] = rec
	}
	seen := make(map[string]struct{}, len(cur))
	for _, rec := range cur {
		seen[rec.Path] = struct{}{}
		was, ok := byPrev[rec.Path]
		if !ok || was.Hash != rec.Hash || was.Size != rec.Size {
			upserts = append(upserts, rec)
		}
	}
	for _, rec := range prev {
		if _, ok := seen[rec.Path]; !ok {
			deletes = append(deletes, rec)
		}
	}
	return upserts, deletes
}

func (t *Tree) Apply(upserts, deletes []Record) *Tree {
	cur := t
	if cur == nil {
		cur = New()
	}
	for _, rec := range deletes {
		cur = cur.Delete(rec.Path)
	}
	for _, rec := range upserts {
		cur = cur.Upsert(rec)
	}
	return cur
}

func (t *Tree) Get(path string) (Record, bool) {
	for n := t.rootOf(); n != nil; {
		switch {
		case path == n.key:
			return n.rec, true
		case path < n.key:
			n = n.left
		default:
			n = n.right
		}
	}
	return Record{}, false
}

func (t *Tree) rootOf() *node {
	if t == nil {
		return nil
	}
	return t.root
}

func upsert(n *node, rec Record) *node {
	if n == nil {
		return leaf(rec)
	}
	if rec.Path == n.key {
		out := *n
		out.rec = rec
		fix(&out)
		return &out
	}
	if rec.Path < n.key {
		out := *n
		out.left = upsert(n.left, rec)
		if out.left.pri > out.pri {
			return rotateRight(&out)
		}
		fix(&out)
		return &out
	}
	out := *n
	out.right = upsert(n.right, rec)
	if out.right.pri > out.pri {
		return rotateLeft(&out)
	}
	fix(&out)
	return &out
}

func remove(n *node, path string) *node {
	if n == nil {
		return nil
	}
	if path < n.key {
		out := *n
		out.left = remove(n.left, path)
		fix(&out)
		return &out
	}
	if path > n.key {
		out := *n
		out.right = remove(n.right, path)
		fix(&out)
		return &out
	}
	return merge(n.left, n.right)
}

func merge(a, b *node) *node {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if a.pri > b.pri {
		out := *a
		out.right = merge(a.right, b)
		fix(&out)
		return &out
	}
	out := *b
	out.left = merge(a, b.left)
	fix(&out)
	return &out
}

func rotateRight(n *node) *node {
	l := *n.left
	n.left = l.right
	fix(n)
	l.right = n
	fix(&l)
	return &l
}

func rotateLeft(n *node) *node {
	r := *n.right
	n.right = r.left
	fix(n)
	r.left = n
	fix(&r)
	return &r
}

func leaf(rec Record) *node {
	n := &node{key: rec.Path, pri: priority(rec.Path), rec: rec, count: 1}
	fix(n)
	return n
}

func fix(n *node) {
	n.count = 1 + countOf(n.left) + countOf(n.right)
	n.sum = hashNode(n)
}

func countOf(n *node) int {
	if n == nil {
		return 0
	}
	return n.count
}

func priority(key string) uint64 {
	sum := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint64(sum[:8])
}

func hashNode(n *node) [32]byte {
	h := sha256.New()
	_, _ = h.Write([]byte("tree2-node\x00"))
	writeChild(h, n.left)
	_, _ = h.Write([]byte(n.key))
	_, _ = h.Write([]byte{0})
	var size [8]byte
	binary.LittleEndian.PutUint64(size[:], n.rec.Size)
	_, _ = h.Write(size[:])
	_, _ = h.Write([]byte(n.rec.Hash))
	_, _ = h.Write([]byte{0})
	writeChild(h, n.right)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func writeChild(h interface{ Write([]byte) (int, error) }, n *node) {
	if n == nil {
		_, _ = h.Write(emptySum[:])
		return
	}
	_, _ = h.Write(n.sum[:])
}

var emptySum = sha256.Sum256([]byte("tree2-empty"))
