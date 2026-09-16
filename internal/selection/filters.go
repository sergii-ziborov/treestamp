package selection

import (
	"encoding/binary"
	"hash"
	"path/filepath"
	"regexp"
	"strings"
)

// Filters are declarative name/dir/regex rules used by FileWalker
// and Scan. Empty filters do not change default oracle selection.
type Filters struct {
	IncludeNames, ExcludeNames           []string
	IncludeDirs, ExcludeDirs             []string
	IncludeNameRegex, ExcludeNameRegex   []*regexp.Regexp
	IncludeDirRegex, ExcludeDirRegex     []*regexp.Regexp
	ExcludeExtensions                    []string
	LocationExclude                      []string
	err                                  error
}

func (f Filters) Err() error { return f.err }

func (f *Filters) SetErr(err error) {
	if err != nil {
		f.err = err
	}
}

func (f Filters) Empty() bool {
	return len(f.IncludeNames)+len(f.ExcludeNames)+len(f.IncludeDirs)+len(f.ExcludeDirs)+
		len(f.IncludeNameRegex)+len(f.ExcludeNameRegex)+len(f.IncludeDirRegex)+len(f.ExcludeDirRegex)+
		len(f.ExcludeExtensions)+len(f.LocationExclude) == 0
}

func (f Filters) needsLocation() bool { return len(f.LocationExclude) > 0 }

func (f Filters) rejectDir(rel, name, joined string) bool {
	if len(f.IncludeDirs) > 0 && !containsFold(f.IncludeDirs, name) && rel != "" {
		return true
	}
	if suffixDirMatch(joined, rel, f.ExcludeDirs) {
		return true
	}
	if len(f.IncludeDirRegex) > 0 && !anyRegex(f.IncludeDirRegex, name) && rel != "" {
		return true
	}
	if anyRegex(f.ExcludeDirRegex, name) {
		return true
	}
	return locationExcluded(joined, f.LocationExclude)
}

func (f Filters) rejectFile(name, joined string) bool {
	if len(f.IncludeNames) > 0 && !containsFold(f.IncludeNames, name) {
		return true
	}
	if containsFold(f.ExcludeNames, name) {
		return true
	}
	if len(f.IncludeNameRegex) > 0 && !anyRegex(f.IncludeNameRegex, name) {
		return true
	}
	if anyRegex(f.ExcludeNameRegex, name) {
		return true
	}
	if excludeExtension(name, f.ExcludeExtensions) {
		return true
	}
	return locationExcluded(joined, f.LocationExclude)
}

func (f Filters) WritePolicy(h hash.Hash) {
	writeFilterList(h, "inc-name", f.IncludeNames)
	writeFilterList(h, "exc-name", f.ExcludeNames)
	writeFilterList(h, "inc-dir", f.IncludeDirs)
	writeFilterList(h, "exc-dir", f.ExcludeDirs)
	writeFilterList(h, "exc-ext", f.ExcludeExtensions)
	writeFilterList(h, "loc", f.LocationExclude)
	writeFilterRegex(h, "inc-name-re", f.IncludeNameRegex)
	writeFilterRegex(h, "exc-name-re", f.ExcludeNameRegex)
	writeFilterRegex(h, "inc-dir-re", f.IncludeDirRegex)
	writeFilterRegex(h, "exc-dir-re", f.ExcludeDirRegex)
}

func containsFold(names []string, name string) bool {
	for _, item := range names {
		if strings.EqualFold(item, name) {
			return true
		}
	}
	return false
}

func anyRegex(exprs []*regexp.Regexp, name string) bool {
	for _, expr := range exprs {
		if expr != nil && expr.MatchString(name) {
			return true
		}
	}
	return false
}

func locationExcluded(joined string, patterns []string) bool {
	slash := filepath.ToSlash(joined)
	for _, pattern := range patterns {
		if pattern != "" && strings.Contains(slash, filepath.ToSlash(pattern)) {
			return true
		}
	}
	return false
}

func suffixDirMatch(joined, rel string, suffixes []string) bool {
	slash := filepath.ToSlash(joined)
	relSlash := filepath.ToSlash(rel)
	for _, suffix := range suffixes {
		suffix = strings.Trim(filepath.ToSlash(suffix), "/")
		if suffix == "" {
			continue
		}
		if relSlash == suffix || strings.HasSuffix(relSlash, "/"+suffix) || strings.HasSuffix(slash, "/"+suffix) || slash == suffix {
			return true
		}
	}
	return false
}

func excludeExtension(name string, exts []string) bool {
	if len(exts) == 0 {
		return false
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	inner := strings.ToLower(strings.TrimPrefix(filepath.Ext(strings.TrimSuffix(name, filepath.Ext(name))), "."))
	for _, item := range exts {
		item = strings.ToLower(strings.TrimPrefix(item, "."))
		if item == ext || item == inner {
			return true
		}
	}
	return false
}

func writeFilterList(h hash.Hash, label string, values []string) {
	writeFilterField(h, label, values)
}

func writeFilterRegex(h hash.Hash, label string, exprs []*regexp.Regexp) {
	values := make([]string, 0, len(exprs))
	for _, expr := range exprs {
		if expr != nil {
			values = append(values, expr.String())
		}
	}
	writeFilterField(h, label, values)
}

func writeFilterField(h hash.Hash, label string, values []string) {
	_, _ = h.Write([]byte{0x1e})
	_, _ = h.Write([]byte(label))
	_, _ = h.Write([]byte{0})
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(values)))
	_, _ = h.Write(hdr[:])
	for _, value := range values {
		binary.LittleEndian.PutUint32(hdr[:], uint32(len(value)))
		_, _ = h.Write(hdr[:])
		_, _ = h.Write([]byte(value))
	}
}
