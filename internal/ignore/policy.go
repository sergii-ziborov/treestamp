package ignore

import (
	"os"
	"path/filepath"
	"strings"
)

// Policy selects ignore sources. The default is repository-local.
type Policy struct {
	ParentRules   bool
	GitIgnore     bool
	DotIgnore     bool
	CustomIgnore  bool
	GitExclude    bool
	GitGlobal     bool
	RequireGit    bool
	ExplicitFiles []string
	set           bool
}

func RepositoryPolicy() Policy {
	return Policy{GitIgnore: true, DotIgnore: true, CustomIgnore: true, set: true}
}

func NonePolicy() Policy { return Policy{set: true} }

func GitCompatiblePolicy() Policy {
	return Policy{
		ParentRules:  true,
		GitIgnore:    true,
		DotIgnore:    true,
		CustomIgnore: true,
		GitExclude:   true,
		GitGlobal:    true,
		RequireGit:   true,
		set:          true,
	}
}

func (p Policy) Specified() bool { return p.set }

func (p Policy) Allows(name string) bool {
	switch name {
	case ".gitignore":
		return p.GitIgnore
	case ".ignore":
		return p.DotIgnore
	default:
		return p.CustomIgnore
	}
}

// Source is one ignore input that participated in selection.
type Source struct {
	Kind        SourceKind
	Location    string
	ContentHash string
}

type SourceKind int

const (
	SourceGitGlobal SourceKind = iota + 1
	SourceGitExclude
	SourceGitIgnore
	SourceDotIgnore
	SourceCustom
	SourceExplicit
	SourceOverride
	SourceGitModules
)

func (k SourceKind) String() string {
	switch k {
	case SourceGitGlobal:
		return "git_global"
	case SourceGitExclude:
		return "git_exclude"
	case SourceGitIgnore:
		return "git_ignore"
	case SourceDotIgnore:
		return "dot_ignore"
	case SourceCustom:
		return "custom"
	case SourceExplicit:
		return "explicit"
	case SourceOverride:
		return "override"
	case SourceGitModules:
		return "git_modules"
	default:
		return ""
	}
}

func (e *Engine) Sources() []Source {
	if e == nil {
		return nil
	}
	out := append([]Source(nil), e.sources...)
	return out
}

func (e *Engine) ApplyPolicy(root string, policy Policy) []string {
	if e == nil {
		return nil
	}
	e.policy = policy
	if policy.RequireGit && !hasGit(root) {
		e.policy.GitIgnore = false
		e.policy.GitExclude = false
		e.policy.GitGlobal = false
		e.policy.ParentRules = false
	}
	var warnings []string
	if e.policy.GitGlobal {
		if path, ok := globalIgnorePath(); ok {
			warnings = append(warnings, e.loadFile(path, "", "", "<git-global>", rankGitGlobal, SourceGitGlobal)...)
		}
	}
	if e.policy.GitExclude {
		path := filepath.Join(root, ".git", "info", "exclude")
		warnings = append(warnings, e.loadFile(path, "", "", ".git/info/exclude", rankGitExclude, SourceGitExclude)...)
	}
	for _, path := range e.policy.ExplicitFiles {
		resolved := path
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(root, resolved)
		}
		warnings = append(warnings, e.loadFile(resolved, "", "", sourceLocation(root, resolved), rankExplicit, SourceExplicit)...)
	}
	if e.policy.ParentRules {
		warnings = append(warnings, e.loadParents(root)...)
	}
	return warnings
}

func (e *Engine) loadParents(root string) []string {
	var warnings []string
	current := filepath.Dir(root)
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return warnings
	}
	seen := map[string]struct{}{}
	for current != "" {
		abs, err := filepath.Abs(current)
		if err != nil || abs == current && current == filepath.Dir(current) {
			if _, ok := seen[current]; ok {
				break
			}
		}
		if _, ok := seen[abs]; ok {
			break
		}
		seen[abs] = struct{}{}
		rel, relErr := filepath.Rel(abs, rootAbs)
		prefix := ""
		if relErr == nil {
			prefix = strings.ReplaceAll(rel, "\\", "/")
			if prefix == "." {
				prefix = ""
			}
		}
		if e.policy.GitIgnore {
			path := filepath.Join(abs, ".gitignore")
			warnings = append(warnings, e.loadFile(path, "", prefix, strings.ReplaceAll(path, "\\", "/"), rankGitIgnore, SourceGitIgnore)...)
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		current = parent
	}
	return warnings
}

func sourceLocation(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return strings.ReplaceAll(relative, "\\", "/")
	}
	return strings.ReplaceAll(path, "\\", "/")
}

func hasGit(root string) bool {
	info, err := os.Lstat(filepath.Join(root, ".git"))
	return err == nil && info != nil
}

func globalIgnorePath() (string, bool) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		path := filepath.Join(xdg, "git", "ignore")
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	candidates := []string{
		filepath.Join(home, ".config", "git", "ignore"),
		filepath.Join(home, ".gitignore"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}
