package report

import "sort"

type Record struct {
	Relative, ContentHash, VersionKey string
	Bytes                             uint64
}

type Change struct {
	Previous, Current Record
}

type Delta struct {
	Added, Removed []Record
	Modified       []Change
	Renamed        []Change
	Unchanged      uint64
}

func Between(prevIn, curIn []Record) Delta {
	var delta Delta
	prev := append([]Record(nil), prevIn...)
	cur := append([]Record(nil), curIn...)
	sort.Slice(prev, func(i, j int) bool { return prev[i].Relative < prev[j].Relative })
	sort.Slice(cur, func(i, j int) bool { return cur[i].Relative < cur[j].Relative })
	i, j := 0, 0
	for i < len(prev) || j < len(cur) {
		switch {
		case i < len(prev) && j < len(cur) && prev[i].Relative == cur[j].Relative:
			if SameContent(prev[i], cur[j]) {
				delta.Unchanged++
			} else {
				delta.Modified = append(delta.Modified, Change{Previous: prev[i], Current: cur[j]})
			}
			i++
			j++
		case j >= len(cur) || (i < len(prev) && prev[i].Relative < cur[j].Relative):
			delta.Removed = append(delta.Removed, prev[i])
			i++
		default:
			delta.Added = append(delta.Added, cur[j])
			j++
		}
	}
	detectRenames(&delta)
	return delta
}

func SameContent(a, b Record) bool {
	if a.Bytes != b.Bytes {
		return false
	}
	if a.ContentHash != "" && b.ContentHash != "" {
		return a.ContentHash == b.ContentHash
	}
	return a.VersionKey != "" && a.VersionKey == b.VersionKey
}

func detectRenames(delta *Delta) {
	prevCounts, curCounts := hashCounts(delta.Removed), hashCounts(delta.Added)
	addedBy, removedBy := uniqueHashIndex(delta.Added), uniqueHashIndex(delta.Removed)
	markAdd, markRem := make([]bool, len(delta.Added)), make([]bool, len(delta.Removed))
	for hash, rem := range removedBy {
		add, ok := addedBy[hash]
		if !ok || prevCounts[hash] != 1 || curCounts[hash] != 1 {
			continue
		}
		markAdd[add], markRem[rem] = true, true
		delta.Renamed = append(delta.Renamed, Change{Previous: delta.Removed[rem], Current: delta.Added[add]})
	}
	sort.Slice(delta.Renamed, func(i, j int) bool {
		if delta.Renamed[i].Previous.Relative != delta.Renamed[j].Previous.Relative {
			return delta.Renamed[i].Previous.Relative < delta.Renamed[j].Previous.Relative
		}
		return delta.Renamed[i].Current.Relative < delta.Renamed[j].Current.Relative
	})
	delta.Added = retainUnmarked(delta.Added, markAdd)
	delta.Removed = retainUnmarked(delta.Removed, markRem)
}

func hashCounts(files []Record) map[string]int {
	out := map[string]int{}
	for _, file := range files {
		if file.ContentHash != "" {
			out[file.ContentHash]++
		}
	}
	return out
}

func uniqueHashIndex(files []Record) map[string]int {
	out, dup := map[string]int{}, map[string]struct{}{}
	for i, file := range files {
		if file.ContentHash == "" {
			continue
		}
		if _, ok := out[file.ContentHash]; ok {
			dup[file.ContentHash] = struct{}{}
			continue
		}
		out[file.ContentHash] = i
	}
	for hash := range dup {
		delete(out, hash)
	}
	return out
}

func retainUnmarked(files []Record, marked []bool) []Record {
	out := files[:0]
	for i, file := range files {
		if !marked[i] {
			out = append(out, file)
		}
	}
	return out
}
