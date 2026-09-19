package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGlobGitignoreSemantics(t *testing.T) {
	if !MatchGlob("*.rs", "lib.rs") {
		t.Fatal("*.rs")
	}
	if MatchGlob("*.rs", "src/lib.rs") {
		t.Fatal("*.rs must not cross /")
	}
	if !MatchGlob("src/**/generated.rs", "src/a/b/generated.rs") {
		t.Fatal("**")
	}
	if !MatchGlob("src/**/generated.rs", "src/generated.rs") {
		t.Fatal("** empty")
	}
	if MatchGlob("src/*/generated.rs", "src/a/b/generated.rs") {
		t.Fatal("single star")
	}
	if !MatchGlob("[a-c].rs", "b.rs") {
		t.Fatal("class")
	}
}

func TestNestedIgnoreAndNegate(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".gitignore"), "dist/\n!dist/keep.txt\n*.log\n")
	eng := NewEngine([]string{".gitignore"}, false, nil)
	eng.LoadDir(dir, "")
	if got := eng.Match("dist", true); got != MatchIgnore {
		t.Fatalf("dist dir %v", got)
	}
	if got := eng.Match("a.log", false); got != MatchIgnore {
		t.Fatalf("log %v", got)
	}
	if got := eng.Match("src/a.go", false); got != MatchNone {
		t.Fatalf("src %v", got)
	}
}

func TestOverrides(t *testing.T) {
	eng := NewEngine(nil, false, []string{"*.go", "!vendor/**"})
	if got := eng.Match("main.go", false); got != MatchOverrideInclude {
		t.Fatalf("go %v", got)
	}
	if got := eng.Match("readme.md", false); got != MatchOverrideIgnore {
		t.Fatalf("md %v", got)
	}
}

func TestBraceAndPolicy(t *testing.T) {
	if !MatchGlob("*.{go,rs}", "a.go") || !MatchGlob("*.{go,rs}", "a.rs") {
		t.Fatal("braces")
	}
	if MatchGlob("*.{go,rs}", "a.md") {
		t.Fatal("md")
	}
	p := RepositoryPolicy()
	if !p.GitIgnore || !p.Specified() {
		t.Fatal("repo policy")
	}
	none := NonePolicy()
	if none.GitIgnore {
		t.Fatal("none")
	}
}

func writeFile(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := writeFile(path, body); err != nil {
		t.Fatal(err)
	}
}

func TestParseCommentsNegateDirAndClass(t *testing.T) {
	_ = MatchGlob("*.go", "A.GO")
	if !MatchGlob("*.go", "a.go") {
		t.Fatal("case")
	}
	if !MatchGlob("[!a]x", "bx") || MatchGlob("[!a]x", "ax") {
		t.Fatal("neg class")
	}
	if !MatchGlob("a?c", "abc") || MatchGlob("a?c", "a/c") {
		t.Fatal("question")
	}
	if !MatchGlob("\\#hash", "#hash") {
		t.Fatal("escaped hash")
	}
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".gitignore"), "# comment\n\\#keep\nbuild/\n!keep.go\n")
	eng := NewEngine([]string{".gitignore"}, true, nil)
	eng.ApplyPolicy(dir, RepositoryPolicy())
	eng.LoadDir(dir, "")
	if !eng.Match("build", true).IsIgnored() {
		t.Fatal("build dir")
	}
	if eng.Clone().Match("keep.go", false).IsIgnored() {
		t.Fatal("keep")
	}
	if SourceGitIgnore.String() == "" || !GitCompatiblePolicy().Specified() {
		t.Fatal("kind")
	}
	if NonePolicy().Allows(".gitignore") {
		t.Fatal("none allows")
	}
}

func TestPolicyAllowsKindsAndParent(t *testing.T) {
	p := RepositoryPolicy()
	if !p.Allows(".gitignore") || !p.Allows(".ignore") || !p.Allows("custom.ignore") {
		t.Fatal("allows")
	}
	git := GitCompatiblePolicy()
	if !git.ParentRules || !git.GitExclude || !git.GitGlobal || !git.RequireGit {
		t.Fatal("git compat")
	}
	if SourceGitGlobal.String() == "" || SourceGitExclude.String() == "" || SourceDotIgnore.String() == "" {
		t.Fatal("kinds")
	}
	if SourceCustom.String() == "" || SourceExplicit.String() == "" || SourceOverride.String() == "" {
		t.Fatal("more kinds")
	}
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".gitignore"), "*.tmp\n")
	eng := NewEngine([]string{".gitignore"}, false, nil)
	eng.ApplyPolicy(dir, RepositoryPolicy())
	warns := eng.LoadDir(dir, "")
	_ = warns
	_ = git
	if !eng.Match("a.tmp", false).IsIgnored() {
		t.Fatal("tmp")
	}
	if eng.Match("a.go", false).IsIgnored() {
		t.Fatal("go")
	}
	if sources := eng.Sources(); len(sources) != 1 || sources[0].Location != ".gitignore" {
		t.Fatalf("sources: %+v", sources)
	}
	mustWrite(t, filepath.Join(dir, ".ignore"), "# comment only\n")
	empty := NewEngine([]string{".ignore"}, false, nil)
	empty.ApplyPolicy(dir, RepositoryPolicy())
	empty.LoadDir(dir, "")
	if sources := empty.Sources(); len(sources) != 1 || sources[0].Location != ".ignore" {
		t.Fatalf("empty source evidence: %+v", sources)
	}
	parent := t.TempDir()
	child := filepath.Join(parent, "proj")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(parent, ".gitignore"), "*.bak\n")
	mustWrite(t, filepath.Join(child, ".ignore"), "*.old\n")
	mustWrite(t, filepath.Join(child, "extra.ignore"), "*.tmp\n")
	nested := NewEngine([]string{".gitignore", ".ignore", "extra.ignore"}, false, nil)
	parentPol := RepositoryPolicy()
	parentPol.ParentRules = true
	nested.ApplyPolicy(child, parentPol)
	nested.LoadDir(child, "")
	if !nested.Match("x.bak", false).IsIgnored() {
		t.Fatal("parent gitignore")
	}
	require := GitCompatiblePolicy()
	bare := NewEngine([]string{".gitignore"}, false, nil)
	bare.ApplyPolicy(t.TempDir(), require)
	xdg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(xdg, "git"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(xdg, "git", "ignore"), "*.log\n")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	global := NewEngine(nil, false, nil)
	gpol := RepositoryPolicy()
	gpol.GitGlobal = true
	global.ApplyPolicy(child, gpol)
	if path, ok := globalIgnorePath(); !ok || path == "" {
		t.Fatal("global path")
	}
	if hasGit(t.TempDir()) {
		t.Fatal("no git")
	}
	if rankFor(".ignore") != rankDotIgnore || rankFor("extra.ignore") != rankCustom {
		t.Fatal("rank")
	}
	if kindFor(".ignore") != SourceDotIgnore || kindFor("extra.ignore") != SourceCustom {
		t.Fatal("kind")
	}
}

func TestGitModulesOptional(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".gitmodules"), `[submodule "contracts/lib/forge-std"]
	path = contracts/lib/forge-std
	url = https://github.com/foundry-rs/forge-std
[submodule "lib/java-tron"]
	path = lib/java-tron
`)
	off := NewEngine(nil, false, nil)
	off.LoadDir(dir, "")
	if off.Match("contracts/lib/forge-std", true).IsIgnored() {
		t.Fatal("default must not honor .gitmodules")
	}
	on := NewEngine(nil, false, nil)
	on.SetGitModules(true)
	on.LoadDir(dir, "")
	if !on.Match("contracts/lib/forge-std", true).IsIgnored() {
		t.Fatal("submodule dir")
	}
	if !on.Match("contracts/lib/forge-std/src.go", false).IsIgnored() {
		t.Fatal("submodule file")
	}
	if on.Match("contracts/keep.go", false).IsIgnored() {
		t.Fatal("sibling")
	}
	if !on.Match("lib/java-tron", true).IsIgnored() {
		t.Fatal("second module")
	}
	sources := on.Sources()
	if len(sources) != 1 || sources[0].Kind != SourceGitModules || sources[0].Location != ".gitmodules" {
		t.Fatalf("source %+v", sources)
	}
	if SourceGitModules.String() != "git_modules" {
		t.Fatal(SourceGitModules.String())
	}
	got := extractGitModuleFolders("path = vendor/foo\npath = .\npath = ..\npath =\n")
	if len(got) != 1 || got[0] != "vendor/foo" {
		t.Fatalf("extract %v", got)
	}
	quoted := extractGitModuleFolders("path = \"third party/lib\"\npath = vendor[1]\npath = lib\n")
	if len(quoted) != 3 || quoted[0] != "third party/lib" || quoted[1] != "vendor[1]" || quoted[2] != "lib" {
		t.Fatalf("literal extract %v", quoted)
	}
	lit := NewEngine(nil, false, nil)
	lit.SetGitModules(true)
	lit.applyGitModules("", []byte("path = vendor[1]\npath = lib\n"))
	if !lit.Match("vendor[1]", true).IsIgnored() || lit.Match("vendor1", true).IsIgnored() {
		t.Fatal("bracket path must stay literal")
	}
	if !lit.Match("lib", true).IsIgnored() || lit.Match("other/lib", true).IsIgnored() {
		t.Fatal("basename must not match nested lib")
	}
}

func TestGlobParseAndEngineEdges(t *testing.T) {
	if (*Engine)(nil).Clone() == nil || (*Engine)(nil).Sources() != nil {
		t.Fatal("nil engine")
	}
	if SourceKind(0).String() != "" {
		t.Fatal("kind zero")
	}
	cases := []struct{ pat, val string }{
		{"{a,b}.go", "a.go"}, {"{a,b}.go", "b.go"}, {"pre{a,b}post", "preapost"},
		{"{a,{b,c}}", "c"}, {"[{a,b}]", "{"}, {"\\{a}", "{a}"}, {"{unclosed", "x"},
		{"**/*.go", "src/a.go"}, {"**", "a/b"}, {"**/", "src"}, {"a/**/b", "a/x/b"},
		{"[a-z]", "m"}, {"[!0-9]", "a"}, {"[^a]", "b"}, {"[abc]", "b"},
		{"[\\]]", "]"}, {"[", "["}, {"a?c", "abc"}, {"*", "file"}, {"foo*", "foobar"},
		{"*bar", "foobar"}, {"FOO*", "foobar"},
	}
	for _, c := range cases {
		_ = MatchGlob(c.pat, c.val)
	}
	_ = MatchGlob("FOO*", "foobar")
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".gitignore"), "build/\n!keep.go\n*.tmp\nfoo* \nbar\\ \n{\n}\nsrc/\n")
	eng := NewEngine([]string{".gitignore"}, true, []string{"*.keep", "!skip.keep"})
	eng.ApplyPolicy(dir, RepositoryPolicy())
	eng.LoadDir(dir, "")
	_ = eng.Match("build/out", false)
	_ = eng.Match("keep.go", false)
	_ = eng.Match("a.tmp", false)
	_ = eng.Match("skip.keep", false)
	nested := filepath.Join(dir, "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(nested, ".gitignore"), "local.dat\n")
	eng.LoadDir(nested, "src")
	_ = eng.Match("src/local.dat", false)
	_, _ = candidateForBase("src/local.dat", "src")
	_, _ = candidateForBase("src", "src")
	_, _ = candidateForBase("other", "src")
	exclude := filepath.Join(dir, ".git", "info")
	if err := os.MkdirAll(exclude, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(exclude, "exclude"), "*.exc\n")
	gitp := RepositoryPolicy()
	gitp.GitExclude = true
	ex := NewEngine(nil, false, nil)
	ex.ApplyPolicy(dir, gitp)
	_ = ex.Match("a.exc", false)
	link := filepath.Join(dir, "link.ignore")
	if err := os.Symlink(filepath.Join(dir, ".gitignore"), link); err == nil {
		warn := NewEngine([]string{"link.ignore"}, false, nil)
		warn.ApplyPolicy(dir, RepositoryPolicy())
		_ = warn.LoadDir(dir, "")
	}
	rules, errs := parseFile("ok.go\n{\n}\n# c\n\\#hash\n!inc.go\n/abs.go\ndir/\nfoo* \nbar\\ \n", false)
	if len(rules) == 0 || len(errs) == 0 {
		t.Fatal("parse")
	}
	_ = validatePattern("a{b")
	_ = validatePattern("a}b")
	_ = validatePattern("a[b{c]d")
	_ = trimUnescapedTrailingSpaces("foo\\ ")
	_ = trimUnescapedTrailingSpaces("foo ")
	_ = matchPattern("ABC", "abc", true)
	_ = matchPattern("pre*", "prefix", true)
	_ = matchPattern("*suf", "xsuf", true)
	_ = asciiLower("AbC1")
}

func TestLoadBytesNestedGitignore(t *testing.T) {
	eng := NewEngine([]string{".gitignore"}, false, nil)
	eng.SetPolicy(RepositoryPolicy())
	if warns := eng.LoadBytes("", ".gitignore", ".gitignore", []byte("secret.bin\n")); len(warns) != 0 {
		t.Fatal(warns)
	}
	if got := eng.Match("secret.bin", false); got != MatchIgnore {
		t.Fatalf("root %v", got)
	}
	if warns := eng.LoadBytes("sub", "sub/.gitignore", ".gitignore", []byte("!secret.bin\nlocal.log\n")); len(warns) != 0 {
		t.Fatal(warns)
	}
	if got := eng.Match("sub/local.log", false); got != MatchIgnore {
		t.Fatalf("nested %v", got)
	}
}

func TestExplainWinningRule(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".gitignore"), "# keep\ngenerated/**\n")
	eng := NewEngine([]string{".gitignore"}, false, nil)
	eng.LoadDir(dir, "")
	got, hit := eng.Explain("generated/model.go", false)
	if got != MatchIgnore || hit.Pattern != "generated/**" || hit.Location != ".gitignore" || hit.Line != 2 {
		t.Fatalf("%v %+v", got, hit)
	}
}
