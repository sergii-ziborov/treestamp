package treestamp

import pathx "github.com/sergii-ziborov/treestamp/internal/path"

// IsSameOrDescendant reports whether path is prefix or a descendant using '/'
// component boundaries.
func IsSameOrDescendant(path, prefix string) bool {
	return pathx.IsSameOrDescendant(path, prefix)
}

// PathCoveredByPrefixes reports whether path is covered by any prefix.
func PathCoveredByPrefixes(path string, prefixes []string) bool {
	return pathx.PathCoveredByPrefixes(path, prefixes)
}

// CollapsePathPrefixes deduplicates and drops prefixes covered by an ancestor.
func CollapsePathPrefixes(prefixes []string) []string {
	return pathx.CollapsePathPrefixes(prefixes)
}
