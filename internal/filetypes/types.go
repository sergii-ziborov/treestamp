package filetypes

import (
	"sort"
	"strings"

	"github.com/sergii-ziborov/treestamp/internal/ignore"
)

// NamedFileTypes is the oracle catalog of reusable name groups.
type NamedFileTypes struct {
	definitions map[string][]string
	selections  []selection
}

type selection struct {
	name    string
	include bool
}

func New() *NamedFileTypes {
	return &NamedFileTypes{definitions: map[string][]string{}}
}

// Defaults loads the 265-name weavatrix-scan catalog. Nothing is selected
// until Select is called.
func Defaults() *NamedFileTypes {
	return New().WithDefaults()
}

func (t *NamedFileTypes) WithDefaults() *NamedFileTypes {
	if t.definitions == nil {
		t.definitions = map[string][]string{}
	}
	for _, def := range catalog {
		for _, name := range def.names {
			if _, ok := t.definitions[name]; !ok {
				t.definitions[name] = append([]string(nil), def.patterns...)
			}
		}
	}
	return t
}

func (t *NamedFileTypes) WithType(name string, extensions ...string) *NamedFileTypes {
	pats := make([]string, 0, len(extensions))
	for _, ext := range extensions {
		ext = strings.TrimPrefix(ext, ".")
		pats = append(pats, "*."+ext)
	}
	if t.definitions == nil {
		t.definitions = map[string][]string{}
	}
	t.definitions[name] = pats
	return t
}

// WithGlobs adds or replaces a named group of arbitrary glob patterns.
func (t *NamedFileTypes) WithGlobs(name string, patterns ...string) *NamedFileTypes {
	if t.definitions == nil {
		t.definitions = map[string][]string{}
	}
	pats := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pats = append(pats, strings.ReplaceAll(pattern, "\\", "/"))
	}
	t.definitions[name] = pats
	return t
}

// WithComposedType adds a type made from existing definition names.
func (t *NamedFileTypes) WithComposedType(name string, components ...string) *NamedFileTypes {
	if t.definitions == nil {
		t.definitions = map[string][]string{}
	}
	var pats []string
	seen := map[string]struct{}{}
	for _, component := range components {
		for _, pattern := range t.definitions[component] {
			if _, ok := seen[pattern]; ok {
				continue
			}
			seen[pattern] = struct{}{}
			pats = append(pats, pattern)
		}
	}
	t.definitions[name] = pats
	return t
}

func (t *NamedFileTypes) Select(names ...string) *NamedFileTypes {
	t.selections = t.selections[:0]
	for _, name := range names {
		t.selections = append(t.selections, selection{name: name, include: true})
	}
	return t
}

func (t *NamedFileTypes) Negate(names ...string) *NamedFileTypes {
	for _, name := range names {
		t.selections = append(t.selections, selection{name: name, include: false})
	}
	return t
}

func (t *NamedFileTypes) Active() bool { return t != nil && len(t.selections) > 0 }

func (t *NamedFileTypes) Len() int {
	if t == nil {
		return 0
	}
	return len(t.definitions)
}

func (t *NamedFileTypes) IsEmpty() bool {
	return t == nil || len(t.definitions) == 0
}

func (t *NamedFileTypes) Names() []string {
	if t == nil {
		return nil
	}
	names := make([]string, 0, len(t.definitions))
	for name := range t.definitions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (t *NamedFileTypes) Contains(name string) bool {
	if t == nil {
		return false
	}
	_, ok := t.definitions[name]
	return ok
}

func (t *NamedFileTypes) Matches(rel, fileName string) (include bool, decided bool) {
	if !t.Active() {
		return false, false
	}
	var last *bool
	for _, sel := range t.selections {
		if matchesAny(t.definitions[sel.name], rel, fileName) {
			v := sel.include
			last = &v
		}
	}
	if last == nil {
		return false, true
	}
	return *last, true
}

func matchesAny(patterns []string, rel, fileName string) bool {
	rel = strings.ReplaceAll(rel, "\\", "/")
	for _, pat := range patterns {
		target := fileName
		if strings.Contains(pat, "/") {
			target = rel
		}
		if matchTypePattern(pat, target) {
			return true
		}
	}
	return false
}

func matchTypePattern(pattern, value string) bool {
	if strings.HasPrefix(pattern, "*.") && !strings.ContainsAny(pattern[2:], "*?[{") {
		return strings.HasSuffix(value, pattern[1:])
	}
	return ignore.MatchGlob(pattern, value)
}
