package pathx

import (
	"path/filepath"
	"sort"
	"strings"
)

// Slash converts a native path to slash form.
func Slash(p string) string {
	return filepath.ToSlash(p)
}

// Native strips a Windows extended-length prefix so Rel and relative
// symlink targets stay in ordinary DOS form. Unix paths are unchanged.
func Native(path string) string {
	const prefix = `\\?\`
	if !strings.HasPrefix(path, prefix) {
		return path
	}
	rest := path[len(prefix):]
	if len(rest) >= 4 && (rest[:4] == `UNC\` || rest[:4] == `unc\`) {
		return `\\` + rest[4:]
	}
	return rest
}

// Resolve evaluates symlinks and returns a native path.
func Resolve(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return Native(resolved), nil
}

// UnderRoot reports whether path is the root or a descendant of root.
// "." after Rel is under root. A Rel of ".." or a parent prefix is not.
func UnderRoot(root, path string) bool {
	rel, err := filepath.Rel(Native(root), Native(path))
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

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

// SafeRelative accepts a repository-relative slash path. "." is the root.
// Absolute paths, empty components, and ".." are rejected.
func SafeRelative(rel string) (string, bool) {
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return ".", true
	}
	if rel == "" || filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return "", false
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." {
			return "", false
		}
	}
	return rel, true
}

func lastSlash(value string) int {
	for i := len(value) - 1; i >= 0; i-- {
		if value[i] == '/' {
			return i
		}
	}
	return -1
}
