// Package selection holds reusable matchers and named types (P2).
package selection

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sergii-ziborov/treestamp/internal/filetypes"
	"github.com/sergii-ziborov/treestamp/internal/ignore"
	pathx "github.com/sergii-ziborov/treestamp/internal/path"
	"github.com/sergii-ziborov/treestamp/internal/platform"
	"github.com/sergii-ziborov/treestamp/internal/walk"
)

type Disposition int

const (
	SelectedFile Disposition = iota
	TraverseDirectory
	Skipped
	Unselected
)

type Decision struct {
	Disposition Disposition
	Skip        SkipKind
	Repo        ignore.Match
}

func (d Decision) IsSelected() bool              { return d.Disposition == SelectedFile }
func (d Decision) ShouldDescend() bool           { return d.Disposition == TraverseDirectory }
func (d Decision) SkipKind() SkipKind            { return d.Skip }
func (d Decision) RepositoryMatch() ignore.Match { return d.Repo }

type SkipKind int

const (
	SkipNone SkipKind = iota
	SkipBinary
	SkipFileSystemBoundary
	SkipExtension
	SkipIgnored
	SkipIOError
	SkipMaxDepth
	SkipOversized
	SkipPathEscape
	SkipStandardDirectory
	SkipHidden
	SkipOverride
	SkipSymlink
	SkipSymlinkLoop
	SkipScanLimit
	SkipConcurrentModification
)

var skipKindNames = map[SkipKind]string{
	SkipBinary:                 "binary",
	SkipFileSystemBoundary:     "filesystem_boundary",
	SkipExtension:              "extension",
	SkipIgnored:                "ignored",
	SkipIOError:                "io_error",
	SkipMaxDepth:               "max_depth",
	SkipOversized:              "oversized",
	SkipPathEscape:             "path_escape",
	SkipStandardDirectory:      "standard_directory",
	SkipHidden:                 "hidden",
	SkipOverride:               "override",
	SkipSymlink:                "symlink",
	SkipSymlinkLoop:            "symlink_loop",
	SkipScanLimit:              "scan_limit",
	SkipConcurrentModification: "concurrent_modification",
}

func (k SkipKind) String() string { return skipKindNames[k] }

var standardDirs = map[string]struct{}{
	".git": {}, ".hg": {}, ".svn": {}, ".venv": {}, "__pycache__": {},
	"build": {}, "coverage": {}, "dist": {}, "node_modules": {},
	"target": {}, "vendor": {},
}

type Config struct {
	IgnoreFiles   []string
	IgnoreCase    bool
	IgnorePolicy  ignore.Policy
	OverrideRules []string
	Extensions    []string
	FileTypes     *filetypes.NamedFileTypes
	SkipHidden    bool
	StandardSkips bool
	MaxFileBytes  uint64
	ApplyMaxBytes bool
	MinDepth      int
	MaxDepth      *int
	FollowLinks   bool
}

type PathQuery struct {
	Rel, Name                    string
	IsDir, IsFile, IsSymlink     bool
	WalkSkip                     walk.WalkSkipReason
	Size                         *uint64
}

type Matcher struct {
	root     string
	engine   *ignore.Engine
	cfg      Config
	warnings []string
}

func NewMatcher(root string, cfg Config) (*Matcher, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	eng := ignore.NewEngine(cfg.IgnoreFiles, cfg.IgnoreCase, cfg.OverrideRules)
	if cfg.IgnorePolicy.Specified() {
		eng.ApplyPolicy(abs, cfg.IgnorePolicy)
	} else {
		eng.ApplyPolicy(abs, ignore.RepositoryPolicy())
	}
	warns := eng.LoadDir(abs, "")
	return &Matcher{root: abs, engine: eng, cfg: cfg, warnings: warns}, nil
}

func (m *Matcher) Sources() []ignore.Source {
	if m == nil {
		return nil
	}
	return m.engine.Sources()
}

func (m *Matcher) DecidePath(q PathQuery) Decision { return m.decide(q) }
func (m *Matcher) Engine() *ignore.Engine          { return m.engine }
func (m *Matcher) EnterDir(abs, rel string) []string {
	return m.engine.LoadDir(abs, pathx.Slash(rel))
}
func (m *Matcher) Decide(entry *walk.WalkEntry) Decision {
	return m.decide(PathQuery{
		Rel: pathx.Slash(entry.RelativePath()), Name: entry.FileName(),
		IsDir: entry.IsDir(), IsFile: entry.IsFile(), IsSymlink: entry.IsSymlink(),
		WalkSkip: entry.SkipReason(), Size: entry.Bytes(),
	})
}
func (m *Matcher) Root() string     { return m.root }
func (m *Matcher) Config() Config   { return m.cfg }
func (m *Matcher) Warnings() []string { return append([]string(nil), m.warnings...) }
func (m *Matcher) MatchedEntry(entry *walk.WalkEntry) Decision { return m.Decide(entry) }

func (m *Matcher) Refresh() (bool, error) {
	next, err := NewMatcher(m.root, m.cfg)
	if err != nil {
		return false, err
	}
	changed := sourcesChanged(m.engine.Sources(), next.engine.Sources())
	*m = *next
	return changed, nil
}

func (m *Matcher) Matched(path string) (Decision, error) {
	rel, err := m.normalize(path)
	if err != nil {
		return Decision{}, err
	}
	if ancestor := m.matchAncestors(rel); ancestor != nil {
		return *ancestor, nil
	}
	abs := filepath.Join(m.root, filepath.FromSlash(rel))
	info, err := os.Lstat(abs)
	if err != nil {
		return Decision{}, err
	}
	isSymlink := info.Mode()&os.ModeSymlink != 0
	if isSymlink && !m.cfg.FollowLinks {
		return Decision{Disposition: Skipped, Skip: SkipSymlink, Repo: ignore.MatchNone}, nil
	}
	if isSymlink {
		target, evalErr := filepath.EvalSymlinks(abs)
		if evalErr != nil {
			return Decision{}, evalErr
		}
		if !pathx.UnderRoot(m.root, target) {
			return Decision{Disposition: Skipped, Skip: SkipPathEscape, Repo: ignore.MatchNone}, nil
		}
		info, err = os.Stat(abs)
		if err != nil {
			return Decision{}, err
		}
	}
	isDir := info.IsDir()
	isFile := info.Mode().IsRegular()
	var size *uint64
	if isFile {
		n := uint64(info.Size())
		size = &n
	}
	return m.decide(PathQuery{Rel: rel, Name: filepath.Base(abs), IsDir: isDir, IsFile: isFile, IsSymlink: isSymlink, Size: size}), nil
}

func (m *Matcher) normalize(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if !pathx.UnderRoot(m.root, abs) {
		return "", os.ErrInvalid
	}
	rel, err := filepath.Rel(m.root, abs)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return "", nil
	}
	return pathx.Slash(rel), nil
}

func (m *Matcher) matchAncestors(rel string) *Decision {
	if rel == "" {
		return nil
	}
	parts := strings.Split(rel, "/")
	prefix := ""
	for i := 0; i < len(parts)-1; i++ {
		if prefix == "" {
			prefix = parts[i]
		} else {
			prefix += "/" + parts[i]
		}
		dec := m.decide(PathQuery{Rel: prefix, Name: parts[i], IsDir: true})
		if dec.Disposition == Skipped {
			return &dec
		}
	}
	return nil
}

func sourcesChanged(a, b []ignore.Source) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i].Kind != b[i].Kind || a[i].Location != b[i].Location || a[i].ContentHash != b[i].ContentHash {
			return true
		}
	}
	return false
}

func (m *Matcher) decide(q PathQuery) Decision {
	if q.WalkSkip != walk.SkipNone {
		return Decision{Disposition: Skipped, Skip: walkSkipKind(q.WalkSkip)}
	}
	if q.IsSymlink && !q.IsFile && !q.IsDir {
		return Decision{Disposition: Skipped, Skip: SkipSymlink, Repo: ignore.MatchNone}
	}
	if m.cfg.SkipHidden && platform.HiddenName(q.Name) && q.Rel != "" {
		return Decision{Disposition: Skipped, Skip: SkipHidden, Repo: ignore.MatchHidden}
	}
	if q.IsDir && m.cfg.StandardSkips {
		if _, ok := standardDirs[q.Name]; ok && q.Rel != "" {
			return Decision{Disposition: Skipped, Skip: SkipStandardDirectory}
		}
	}
	repo := m.engine.Match(q.Rel, q.IsDir)
	if repo.IsIgnored() {
		skip := SkipIgnored
		if repo == ignore.MatchOverrideIgnore {
			skip = SkipOverride
		} else if repo == ignore.MatchHidden {
			skip = SkipHidden
		}
		return Decision{Disposition: Skipped, Skip: skip, Repo: repo}
	}
	return m.decideSelected(q, repo)
}

func (m *Matcher) decideSelected(q PathQuery, repo ignore.Match) Decision {
	depth := relativeDepth(q.Rel)
	if m.cfg.MaxDepth != nil && depth > *m.cfg.MaxDepth {
		return Decision{Disposition: Skipped, Skip: SkipMaxDepth, Repo: repo}
	}
	if q.IsDir {
		return Decision{Disposition: TraverseDirectory, Repo: repo}
	}
	if !q.IsFile || (m.cfg.MinDepth > 0 && depth < m.cfg.MinDepth) {
		return Decision{Disposition: Unselected, Repo: repo}
	}
	if !m.extensionOK(q.Rel, q.Name) {
		return Decision{Disposition: Skipped, Skip: SkipExtension, Repo: repo}
	}
	if m.cfg.FileTypes != nil && m.cfg.FileTypes.Active() {
		if include, decided := m.cfg.FileTypes.Matches(q.Rel, q.Name); decided && !include {
			return Decision{Disposition: Skipped, Skip: SkipExtension, Repo: repo}
		}
	}
	if m.cfg.ApplyMaxBytes && m.cfg.MaxFileBytes > 0 && q.Size != nil && *q.Size > m.cfg.MaxFileBytes {
		return Decision{Disposition: Skipped, Skip: SkipOversized, Repo: repo}
	}
	return Decision{Disposition: SelectedFile, Repo: repo}
}

func (m *Matcher) extensionOK(rel, name string) bool {
	if len(m.cfg.Extensions) == 0 {
		return true
	}
	lower := strings.ToLower(name)
	for _, ext := range m.cfg.Extensions {
		ext = strings.ToLower(strings.TrimPrefix(ext, "."))
		if strings.HasSuffix(lower, "."+ext) || strings.EqualFold(name, ext) {
			return true
		}
	}
	_ = rel
	return false
}

func walkSkipKind(reason walk.WalkSkipReason) SkipKind {
	switch reason {
	case walk.SkipMaxDepth:
		return SkipMaxDepth
	case walk.SkipFileSystemBoundary:
		return SkipFileSystemBoundary
	case walk.SkipPathEscape:
		return SkipPathEscape
	case walk.SkipSymlinkLoop:
		return SkipSymlinkLoop
	default:
		return SkipNone
	}
}

func relativeDepth(rel string) int {
	if rel == "" {
		return 0
	}
	return strings.Count(rel, "/") + 1
}
