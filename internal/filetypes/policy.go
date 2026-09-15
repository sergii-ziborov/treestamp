package filetypes

import (
	"hash"
	"sort"
	"strings"
)

type policyPattern struct {
	kind  byte
	value string
}

// WritePolicy appends the oracle descriptor feed for named types.
func (t *NamedFileTypes) WritePolicy(h hash.Hash) {
	if t != nil {
		names := make([]string, 0, len(t.definitions))
		for name := range t.definitions {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			_, _ = h.Write([]byte(name))
			_, _ = h.Write([]byte{0})
			patterns := policyPatterns(t.definitions[name])
			sort.Slice(patterns, func(i, j int) bool {
				if patterns[i].kind != patterns[j].kind {
					return patterns[i].kind < patterns[j].kind
				}
				return patterns[i].value < patterns[j].value
			})
			for _, pat := range patterns {
				_, _ = h.Write([]byte{pat.kind})
				_, _ = h.Write([]byte(pat.value))
				_, _ = h.Write([]byte{0})
			}
			_, _ = h.Write([]byte{0xfe})
		}
		for _, sel := range t.selections {
			if sel.include {
				_, _ = h.Write([]byte{1})
			} else {
				_, _ = h.Write([]byte{2})
			}
			_, _ = h.Write([]byte(sel.name))
			_, _ = h.Write([]byte{0xff})
		}
	}
}

func policyPatterns(patterns []string) []policyPattern {
	out := make([]policyPattern, 0, len(patterns))
	for _, pattern := range patterns {
		normalized := strings.ReplaceAll(pattern, "\\", "/")
		if ext, ok := extensionPattern(normalized); ok {
			out = append(out, policyPattern{kind: 1, value: ext})
			continue
		}
		out = append(out, policyPattern{kind: 2, value: normalized})
	}
	return out
}

func extensionPattern(pattern string) (string, bool) {
	rest, ok := strings.CutPrefix(pattern, "*.")
	if !ok {
		return "", false
	}
	if strings.Contains(rest, ".") {
		return "", false
	}
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '*', '?', '[', '{', '/':
			return "", false
		}
	}
	return strings.ToLower(strings.TrimLeft(rest, ".")), true
}
