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
	base     string
	prefix   string
	location string
	rules    []ignoreRule
	parent   *layer
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
	modules         []string
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

// SetPolicy records ignore-source flags without reading OS policy files.
func (e *Engine) SetPolicy(policy Policy) {
	if e != nil {
		e.policy = policy
	}
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

// LoadDirListed loads ignore files that already appear in a directory listing.
func (e *Engine) LoadDirListed(directory, base string, dents []os.DirEntry) ([]string, bool) {
	var warnings []string
	loaded := false
	for _, dent := range dents {
		name := dent.Name()
		if !e.listedIgnore(name) {
			continue
		}
		loaded = true
		location := name
		if base != "" {
			location = base + "/" + name
		}
		if name == ".gitmodules" {
			warnings = append(warnings, e.loadGitModules(directory, base)...)
			continue
		}
		warnings = append(warnings, e.loadFile(filepath.Join(directory, name), base, "", location, rankFor(name), kindFor(name))...)
	}
	return warnings, loaded
}

// Idle reports that no ignore/override rules are loaded.
func (e *Engine) Idle() bool {
	if e == nil || e.hasIncludes || len(e.overrides) > 0 || len(e.sources) > 0 || len(e.modules) > 0 {
		return false
	}
	for i := range e.layers {
		if e.layers[i] != nil {
			return false
		}
	}
	return true
}

func (e *Engine) listedIgnore(name string) bool {
	if name == ".gitmodules" {
		return e.gitModules
	}
	for _, file := range e.files {
		if file == name && e.policy.Allows(name) {
			return true
		}
	}
	return false
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
	return e.applyRules(base, prefix, location, rank, kind, body)
}

// LoadBytes applies one ignore file already read from an fs.FS or test fixture.
func (e *Engine) LoadBytes(base, location, name string, body []byte) []string {
	if e == nil {
		return nil
	}
	if name == ".gitmodules" {
		if !e.gitModules {
			return nil
		}
		return e.applyGitModules(base, body)
	}
	if !e.policy.Allows(name) {
		return nil
	}
	return e.applyRules(base, "", location, rankFor(name), kindFor(name), body)
}

func (e *Engine) applyRules(base, prefix, location string, rank int, kind SourceKind, body []byte) []string {
	rules, errs := parseFile(string(body), e.caseInsensitive)
	if len(rules) > 0 {
		e.layers[rank] = &layer{base: base, prefix: prefix, location: location, rules: rules, parent: e.layers[rank]}
	}
	e.sources = append(e.sources, Source{
		Kind: kind, Location: strings.ReplaceAll(location, "\\", "/"),
		ContentHash: hashx.SHA256Prefix(body),
	})
	return errs
}

// Hit is the winning rule when one is known.
type Hit struct {
	Pattern, Location string
	Line              int
}

// Match reports the winning ignore/override decision.
func (e *Engine) Match(rel string, isDir bool) Match {
	m, _ := e.Explain(rel, isDir)
	return m
}

func (e *Engine) Explain(rel string, isDir bool) (Match, Hit) {
	if strings.IndexByte(rel, '\\') >= 0 {
		rel = strings.ReplaceAll(rel, "\\", "/")
	}
	if ov, hit, ok := e.explainOverrides(rel, isDir); ok {
		return ov, hit
	}
	if e.matchModule(rel) {
		return MatchIgnore, Hit{Pattern: rel, Location: ".gitmodules"}
	}
	if action, hit, ok := e.explainRules(rel, isDir); ok {
		if action == actionIgnore {
			return MatchIgnore, hit
		}
		return MatchInclude, hit
	}
	return MatchNone, Hit{}
}

func (e *Engine) matchModule(rel string) bool {
	for _, prefix := range e.modules {
		if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
			return true
		}
	}
	return false
}

func (e *Engine) explainOverrides(rel string, isDir bool) (Match, Hit, bool) {
	if len(e.overrides) == 0 {
		return MatchNone, Hit{}, false
	}
	if rule := bestRule(e.overrides, rel, isDir); rule != nil {
		hit := Hit{Pattern: rule.pattern, Location: "<override>", Line: rule.line}
		if rule.action == actionIgnore {
			return MatchOverrideIgnore, hit, true
		}
		return MatchOverrideInclude, hit, true
	}
	if e.hasIncludes && !isDir {
		return MatchOverrideIgnore, Hit{Location: "<override>"}, true
	}
	return MatchNone, Hit{}, false
}

func (e *Engine) matchOverrides(rel string, isDir bool) Match {
	m, _, ok := e.explainOverrides(rel, isDir)
	if !ok {
		return MatchNone
	}
	return m
}

func (e *Engine) explainRules(path string, isDir bool) (ruleAction, Hit, bool) {
	ancestorIncluded := false
	var ancestorHit Hit
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
			rm, found := matchSet(current.rules, candidate, isDir)
			if !found {
				continue
			}
			hit := Hit{Pattern: rm.rule.pattern, Location: current.location, Line: rm.rule.line}
			switch rm.kind {
			case matchExact:
				return rm.action, hit, true
			case matchAncestor:
				if rm.action == actionInclude {
					ancestorIncluded, ancestorHit = true, hit
				} else if !ancestorIncluded {
					return actionIgnore, hit, true
				}
			}
		}
	}
	if ancestorIncluded {
		return actionInclude, ancestorHit, true
	}
	return 0, Hit{}, false
}

func bestRule(rules []ignoreRule, path string, isDir bool) *ignoreRule {
	for i := len(rules) - 1; i >= 0; i-- {
		if rules[i].matchesExact(path, isDir) {
			return &rules[i]
		}
	}
	return nil
}

func (e *Engine) matchRules(path string, isDir bool) (ruleAction, bool) {
	action, _, ok := e.explainRules(path, isDir)
	return action, ok
}

type matchKind int

const (
	matchExact matchKind = iota
	matchAncestor
)

type setMatch struct {
	kind   matchKind
	action ruleAction
	rule   ignoreRule
}

func matchSet(rules []ignoreRule, path string, isDir bool) (setMatch, bool) {
	if rule := bestRule(rules, path, isDir); rule != nil {
		return setMatch{kind: matchExact, action: rule.action, rule: *rule}, true
	}
	ancestor := path
	for {
		slash := strings.LastIndexByte(ancestor, '/')
		if slash < 0 {
			return setMatch{}, false
		}
		ancestor = ancestor[:slash]
		if rule := bestRule(rules, ancestor, true); rule != nil {
			return setMatch{kind: matchAncestor, action: rule.action, rule: *rule}, true
		}
	}
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
