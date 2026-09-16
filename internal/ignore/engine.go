// Package ignore holds nested ignore sources and precedence (P2).
package ignore

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sergii-ziborov/treestamp/internal/hashx"
)

// Match is the highest-precedence repository decision for one path.
type Match int

const (
	MatchNone Match = iota
	MatchIgnore
	MatchInclude
	MatchOverrideIgnore
	MatchOverrideInclude
	MatchHidden
)

func (m Match) IsIgnored() bool {
	return m == MatchIgnore || m == MatchOverrideIgnore || m == MatchHidden
}

const sourceCount = 7

const (
	rankGitGlobal = iota
	rankGitExclude
	rankGitIgnore
	rankDotIgnore
	rankCustom
	rankExplicit
	rankGitModules
)

type layer struct {
	base   string
	prefix string
	rules  []ignoreRule
	parent *layer
}

// Engine holds nested ignore sources and override globs.
type Engine struct {
	files           []string
	caseInsensitive bool
	layers          [sourceCount]*layer
	overrides       []ignoreRule
	hasIncludes     bool
	policy          Policy
	sources         []Source
	gitModules      bool
}

// NewEngine builds a matcher. Call LoadDir as directories are entered.
func NewEngine(ignoreFiles []string, caseInsensitive bool, overrides []string) *Engine {
	eng := &Engine{files: append([]string(nil), ignoreFiles...), caseInsensitive: caseInsensitive}
	if len(overrides) > 0 {
		rules, _ := parseFile(strings.Join(overrides, "\n"), caseInsensitive)
		for i := range rules {
			if rules[i].action == actionIgnore {
				rules[i].action = actionInclude
			} else {
				rules[i].action = actionIgnore
			}
			if rules[i].action == actionInclude {
				eng.hasIncludes = true
			}
		}
		eng.overrides = rules
		eng.sources = append(eng.sources, Source{
			Kind:        SourceOverride,
			Location:    "<override>",
			ContentHash: hashx.SHA256Prefix([]byte(strings.Join(overrides, "\n"))),
		})
	}
	eng.policy = RepositoryPolicy()
	return eng
}

func (e *Engine) SetGitModules(enabled bool) {
	if e != nil {
		e.gitModules = enabled
	}
}

// Clone returns a copy that can load additional nested directories.
func (e *Engine) Clone() *Engine {
	if e == nil {
		return NewEngine(nil, false, nil)
	}
	out := *e
	return &out
}

// LoadDir reads ignore files in directory. base is the slash-relative path of that directory.
func (e *Engine) LoadDir(directory, base string) []string {
	var warnings []string
	for _, name := range e.files {
		if !e.policy.Allows(name) {
			continue
		}
		location := name
		if base != "" {
			location = base + "/" + name
		}
		warnings = append(warnings, e.loadFile(filepath.Join(directory, name), base, "", location, rankFor(name), kindFor(name))...)
	}
	if e.gitModules {
		warnings = append(warnings, e.loadGitModules(directory, base)...)
	}
	return warnings
}

func (e *Engine) loadFile(path, base, prefix, location string, rank int, kind SourceKind) []string {
	info, err := os.Lstat(path)
	if err != nil {
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return []string{path + ": ignore file is a symbolic link"}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return []string{err.Error()}
	}
	rules, errs := parseFile(string(body), e.caseInsensitive)
	if len(rules) > 0 {
		e.layers[rank] = &layer{base: base, prefix: prefix, rules: rules, parent: e.layers[rank]}
	}
	e.sources = append(e.sources, Source{
		Kind: kind, Location: strings.ReplaceAll(location, "\\", "/"),
		ContentHash: hashx.SHA256Prefix(body),
	})
	return errs
}

// Match reports the winning ignore/override decision.
func (e *Engine) Match(rel string, isDir bool) Match {
	rel = strings.ReplaceAll(rel, "\\", "/")
	if ov := e.matchOverrides(rel, isDir); ov != MatchNone {
		return ov
	}
	if action, ok := e.matchRules(rel, isDir); ok {
		if action == actionIgnore {
			return MatchIgnore
		}
		return MatchInclude
	}
	return MatchNone
}

func (e *Engine) matchOverrides(rel string, isDir bool) Match {
	if len(e.overrides) == 0 {
		return MatchNone
	}
	if action, ok := bestExact(e.overrides, rel, isDir); ok {
		if action == actionIgnore {
			return MatchOverrideIgnore
		}
		return MatchOverrideInclude
	}
	if e.hasIncludes && !isDir {
		return MatchOverrideIgnore
	}
	return MatchNone
}

func (e *Engine) matchRules(path string, isDir bool) (ruleAction, bool) {
	ancestorIncluded := false
	for rank := sourceCount - 1; rank >= 0; rank-- {
		for current := e.layers[rank]; current != nil; current = current.parent {
			lookup := path
			if current.prefix != "" {
				lookup = current.prefix + "/" + path
			}
			candidate, ok := candidateForBase(lookup, current.base)
			if !ok {
				continue
			}
			if rm, found := matchSet(current.rules, candidate, isDir); found {
				switch rm.kind {
				case matchExact:
					return rm.action, true
				case matchAncestor:
					if rm.action == actionInclude {
						ancestorIncluded = true
					} else if !ancestorIncluded {
						return actionIgnore, true
					}
				}
			}
		}
	}
	if ancestorIncluded {
		return actionInclude, true
	}
	return 0, false
}

type matchKind int

const (
	matchExact matchKind = iota
	matchAncestor
)

type setMatch struct {
	kind   matchKind
	action ruleAction
}

func matchSet(rules []ignoreRule, path string, isDir bool) (setMatch, bool) {
	if action, ok := bestExact(rules, path, isDir); ok {
		return setMatch{kind: matchExact, action: action}, true
	}
	ancestor := path
	for {
		slash := strings.LastIndexByte(ancestor, '/')
		if slash < 0 {
			return setMatch{}, false
		}
		ancestor = ancestor[:slash]
		if action, ok := bestExact(rules, ancestor, true); ok {
			return setMatch{kind: matchAncestor, action: action}, true
		}
	}
}

func bestExact(rules []ignoreRule, path string, isDir bool) (ruleAction, bool) {
	for i := len(rules) - 1; i >= 0; i-- {
		if rules[i].matchesExact(path, isDir) {
			return rules[i].action, true
		}
	}
	return 0, false
}

func candidateForBase(path, base string) (string, bool) {
	if base == "" {
		return path, true
	}
	if path == base {
		return "", false
	}
	prefix := base + "/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	return path[len(prefix):], true
}

func rankFor(name string) int {
	switch name {
	case ".gitignore":
		return rankGitIgnore
	case ".ignore":
		return rankDotIgnore
	default:
		return rankCustom
	}
}

func kindFor(name string) SourceKind {
	switch name {
	case ".gitignore":
		return SourceGitIgnore
	case ".ignore":
		return SourceDotIgnore
	default:
		return SourceCustom
	}
}
