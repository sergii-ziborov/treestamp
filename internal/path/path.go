package pathx

import "sort"

// IsSameOrDescendant reports whether path is prefix or a descendant using
// '/' component boundaries. "src" matches "src/a.rs" and does not match "src2".
func IsSameOrDescendant(path, prefix string) bool {
	if path == prefix {
		return true
	}
	if prefix == "" {
		return false
	}
	if len(path) <= len(prefix) {
		return false
	}
	if path[len(prefix)] != '/' {
		return false
	}
	return path[:len(prefix)] == prefix
}

// PathCoveredByPrefixes reports whether path equals or sits under any prefix.
func PathCoveredByPrefixes(path string, prefixes []string) bool {
	set := make(map[string]struct{}, len(prefixes))
	for _, prefix := range prefixes {
		set[prefix] = struct{}{}
	}
	if _, ok := set[path]; ok {
		return true
	}
	rest := path
	for {
		slash := lastSlash(rest)
		if slash < 0 {
			return false
		}
		parent := rest[:slash]
		if _, ok := set[parent]; ok {
			return true
		}
		rest = parent
	}
}

// CollapsePathPrefixes deduplicates and drops prefixes covered by an ancestor.
func CollapsePathPrefixes(prefixes []string) []string {
	sorted := append([]string(nil), prefixes...)
	sort.Strings(sorted)
	out := make([]string, 0, len(sorted))
	seen := make(map[string]struct{})
	for _, prefix := range sorted {
		if _, ok := seen[prefix]; ok {
			continue
		}
		seen[prefix] = struct{}{}
		if PathCoveredByPrefixes(prefix, out) {
			continue
		}
		out = append(out, prefix)
	}
	return out
}

func lastSlash(value string) int {
	for i := len(value) - 1; i >= 0; i-- {
		if value[i] == '/' {
			return i
		}
	}
	return -1
}
