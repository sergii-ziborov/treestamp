package selection

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/sergii-ziborov/treestamp/internal/filetypes"
	"github.com/sergii-ziborov/treestamp/internal/walk"
)

func TestDecideHiddenStandardExtension(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "keep.go"), "package k")
	mustWrite(t, filepath.Join(root, ".hidden.go"), "package h")
	mustMkdir(t, filepath.Join(root, "node_modules"))
	mustWrite(t, filepath.Join(root, "node_modules", "x.js"), "x")
	mustWrite(t, filepath.Join(root, "notes.md"), "md")
	m, err := NewMatcher(root, Config{SkipHidden: true, StandardSkips: true, Extensions: []string{"go"}})
	if err != nil {
		t.Fatal(err)
	}
	if !m.DecidePath(PathQuery{Rel: "keep.go", Name: "keep.go", IsFile: true}).IsSelected() {
		t.Fatal("keep")
	}
	if m.DecidePath(PathQuery{Rel: ".hidden.go", Name: ".hidden.go", IsFile: true}).Skip != SkipHidden {
		t.Fatal("hidden")
	}
	if m.DecidePath(PathQuery{Rel: "node_modules", Name: "node_modules", IsDir: true}).Skip != SkipStandardDirectory {
		t.Fatal("standard")
	}
	if m.DecidePath(PathQuery{Rel: "notes.md", Name: "notes.md", IsFile: true}).Skip != SkipExtension {
		t.Fatal("ext")
	}
	if SkipHidden.String() != "hidden" || SkipNone.String() != "" {
		t.Fatal(SkipHidden.String())
	}
}

func TestMatchedAndRefresh(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".gitignore"), "skip.txt\n")
	mustWrite(t, filepath.Join(root, "keep.go"), "package k")
	mustWrite(t, filepath.Join(root, "skip.txt"), "no")
	mustWrite(t, filepath.Join(root, ".gitmodules"), "path = extra\n")
	mustMkdir(t, filepath.Join(root, "extra"))
	mustWrite(t, filepath.Join(root, "extra", "lib.go"), "package extra")
	m, err := NewMatcher(root, Config{IgnoreFiles: []string{".gitignore"}, GitModules: true})
	if err != nil {
		t.Fatal(err)
	}
	keep, err := m.Matched(filepath.Join(root, "keep.go"))
	if err != nil || !keep.IsSelected() {
		t.Fatalf("keep %+v %v", keep, err)
	}
	skip, err := m.Matched(filepath.Join(root, "skip.txt"))
	if err != nil || skip.SkipKind() != SkipIgnored {
		t.Fatalf("skip %+v %v", skip, err)
	}
	changed, err := m.Refresh()
	if err != nil || changed {
		t.Fatalf("refresh %v %v", changed, err)
	}
	if m.Root() == "" || m.Engine() == nil {
		t.Fatal("accessors")
	}
	if _, err := m.Matched(filepath.Join(root, "..", "outside")); err == nil {
		t.Fatal("outside")
	}
}

func TestLoadPathScopeNestedIgnore(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWrite(t, filepath.Join(root, "sub", ".gitignore"), "*.tmp\n")
	mustWrite(t, filepath.Join(root, "sub", "drop.tmp"), "x")
	m, err := NewMatcher(root, Config{IgnoreFiles: []string{".gitignore"}})
	if err != nil {
		t.Fatal(err)
	}
	q := PathQuery{Rel: "sub/drop.tmp", Name: "drop.tmp", IsFile: true}
	if m.DecidePath(q).IsSelected() == false {
		t.Fatal("root-only matcher should not yet see nested ignore")
	}
	_ = m.LoadPathScope("sub/drop.tmp")
	dec := m.DecideWithAncestors(q)
	if dec.IsSelected() || dec.Skip != SkipIgnored {
		t.Fatalf("nested ignore %+v", dec)
	}
}

func TestWalkSkipAndFileTypes(t *testing.T) {
	root := t.TempDir()
	m, err := NewMatcher(root, Config{
		FileTypes: filetypes.New().WithGlobs("go", "*.go").Select("go"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.DecidePath(PathQuery{Rel: "a", Name: "a", IsDir: true, WalkSkip: walk.SkipMaxDepth}).Skip != SkipMaxDepth {
		t.Fatal("walk skip")
	}
	if m.DecidePath(PathQuery{Rel: "link", Name: "link", IsSymlink: true}).Skip != SkipSymlink {
		t.Fatal("symlink")
	}
	if !m.DecidePath(PathQuery{Rel: "a.go", Name: "a.go", IsFile: true}).IsSelected() {
		t.Fatal("go type")
	}
	if m.DecidePath(PathQuery{Rel: "a.md", Name: "a.md", IsFile: true}).Skip != SkipExtension {
		t.Fatal("md type")
	}
	size := uint64(100)
	m.cfg.ApplyMaxBytes = true
	m.cfg.MaxFileBytes = 10
	if m.DecidePath(PathQuery{Rel: "a.go", Name: "a.go", IsFile: true, Size: &size}).Skip != SkipOversized {
		t.Fatal("size")
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestDecideAccessorsAndOverrides(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a")
	m, err := NewMatcher(root, Config{OverrideRules: []string{"*.go"}, FollowLinks: true})
	if err != nil {
		t.Fatal(err)
	}
	dec := m.DecidePath(PathQuery{Rel: "a.go", Name: "a.go", IsFile: true})
	if !dec.IsSelected() || dec.ShouldDescend() {
		t.Fatalf("%+v", dec)
	}
	_ = dec.RepositoryMatch()
	_ = m.Sources()
	if (*Matcher)(nil).Sources() != nil {
		t.Fatal("nil sources")
	}
	_ = m.Warnings()
	_ = m.Config()
	_ = m.EnterDir(root, "")
	_ = m.MatchedEntry(&walk.WalkEntry{})
	if m.DecidePath(PathQuery{Rel: "x.md", Name: "x.md", IsFile: true}).Skip != SkipOverride {
		t.Fatal("override")
	}
	if walkSkipKind(walk.SkipFileSystemBoundary) != SkipFileSystemBoundary {
		t.Fatal("map")
	}
}

func TestMinDepthAncestorsAndWalkSkips(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWrite(t, filepath.Join(root, "sub", "a.go"), "package a")
	max := 1
	m, err := NewMatcher(root, Config{MinDepth: 2, MaxDepth: &max, FollowLinks: false})
	if err != nil {
		t.Fatal(err)
	}
	if m.DecidePath(PathQuery{Rel: "sub/a.go", Name: "a.go", IsFile: true}).IsSelected() {
		t.Fatal("max depth file")
	}
	dec := m.DecidePath(PathQuery{Rel: "sub", Name: "sub", IsDir: true})
	if !dec.ShouldDescend() && dec.Skip != SkipMaxDepth {
		t.Fatalf("dir %+v", dec)
	}
	if walkSkipKind(walk.SkipPathEscape) != SkipPathEscape || walkSkipKind(walk.SkipSymlinkLoop) != SkipSymlinkLoop {
		t.Fatal("walk map")
	}
	if SkipBinary.String() == "" || SkipIOError.String() == "" || SkipOversized.String() == "" {
		t.Fatal("strings")
	}
	deep, err := NewMatcher(root, Config{MinDepth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if deep.DecidePath(PathQuery{Rel: "a.go", Name: "a.go", IsFile: true}).IsSelected() {
		t.Fatal("min depth file")
	}
	if !deep.DecidePath(PathQuery{Rel: "sub", Name: "sub", IsDir: true}).ShouldDescend() {
		t.Fatal("min depth dir")
	}
	if _, err := deep.Matched(filepath.Join(root, "missing.go")); err == nil {
		t.Fatal("missing")
	}
	_, _ = deep.Refresh()
	_ = walkSkipKind(walk.SkipNone)
	_ = walkSkipKind(walk.SkipMaxDepth)
	_ = relativeDepth("")
	_ = relativeDepth("a/b")
}

func TestDeclarativeFilters(t *testing.T) {
	root := t.TempDir()
	m, err := NewMatcher(root, Config{Filters: Filters{
		IncludeNameRegex: []*regexp.Regexp{regexp.MustCompile(`\.go$`)},
		ExcludeNames:     []string{"skip.go"},
		ExcludeDirs:      []string{"vendor"},
		LocationExclude:  []string{"zz-drop"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !m.DecidePath(PathQuery{Rel: "keep.go", Name: "keep.go", IsFile: true}).IsSelected() {
		t.Fatal("keep")
	}
	if m.DecidePath(PathQuery{Rel: "skip.go", Name: "skip.go", IsFile: true}).IsSelected() {
		t.Fatal("exclude name")
	}
	if m.DecidePath(PathQuery{Rel: "notes.md", Name: "notes.md", IsFile: true}).IsSelected() {
		t.Fatal("regex")
	}
	if m.DecidePath(PathQuery{Rel: "vendor", Name: "vendor", IsDir: true}).ShouldDescend() {
		t.Fatal("exclude dir")
	}
	if m.DecidePath(PathQuery{Rel: "zz-drop/a.go", Name: "a.go", IsFile: true}).IsSelected() {
		t.Fatal("location")
	}
}

func TestCompiledSelectionMayContain(t *testing.T) {
	plan := Compile(Config{OverrideRules: []string{"services/payments/**/*.go", "libs/contracts/**/*.proto"}})
	if plan.MayContain("services/payments/api.go") != ContainMaybe {
		t.Fatal("hit")
	}
	if plan.MayContain("services") != ContainMaybe {
		t.Fatal("parent")
	}
	if plan.MayContain("vendor") != ContainNo {
		t.Fatal("vendor")
	}
	if Compile(Config{OverrideRules: []string{"**/*.go"}}).MayContain("vendor") != ContainMaybe {
		t.Fatal("unbounded")
	}
	if Compile(Config{}).MayContain("anywhere") != ContainMaybe {
		t.Fatal("open")
	}
}
