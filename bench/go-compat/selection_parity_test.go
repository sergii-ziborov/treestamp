package compat_test

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/boyter/gocodewalker"
	"github.com/sergii-ziborov/treestamp"
)

func TestRepositoryIgnoreSelectionParity(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".gitignore"), "*.tmp\n!keep.tmp\nbuild/\n")
	write(t, filepath.Join(root, ".ignore"), "*.log\n")
	write(t, filepath.Join(root, ".weavatrixignore"), "*.secret\n")
	write(t, filepath.Join(root, "visible.go"), "package visible")
	write(t, filepath.Join(root, ".hidden.go"), "package hidden")
	write(t, filepath.Join(root, "drop.tmp"), "drop")
	write(t, filepath.Join(root, "keep.tmp"), "keep")
	write(t, filepath.Join(root, "app.log"), "log")
	write(t, filepath.Join(root, "token.secret"), "secret")
	write(t, filepath.Join(root, "build", "generated.go"), "package generated")
	write(t, filepath.Join(root, "nested", ".gitignore"), "*.gen\n")
	write(t, filepath.Join(root, "nested", "drop.gen"), "drop")
	write(t, filepath.Join(root, "nested", "keep.go"), "package keep")

	tree, err := treestampPaths(root, func(opts treestamp.Options) treestamp.Options {
		return opts.WithIgnoreFiles(".gitignore", ".ignore", ".weavatrixignore")
	})
	if err != nil {
		t.Fatal(err)
	}
	code, err := codewalkerPaths(root, func(w *gocodewalker.FileWalker) {
		w.IncludeHidden = true
		w.CustomIgnore = []string{".weavatrixignore"}
		w.IgnoreGitModules = true
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, tree, map[string][]string{"gocodewalker": code})
}

func TestExtensionSelectionParity(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a.go"), "package a")
	write(t, filepath.Join(root, "b.txt"), "b")
	write(t, filepath.Join(root, ".hidden.go"), "package hidden")
	write(t, filepath.Join(root, "nested", "c.go"), "package c")

	tree, err := treestampPaths(root, func(opts treestamp.Options) treestamp.Options {
		return opts.WithExtensions("go")
	})
	if err != nil {
		t.Fatal(err)
	}
	code, err := codewalkerPaths(root, func(w *gocodewalker.FileWalker) {
		w.AllowListExtensions = []string{"go"}
		w.IncludeHidden = true
		w.IgnoreGitModules = true
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, tree, map[string][]string{"gocodewalker": code})
}

func TestHiddenSelectionSemantics(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "visible.go"), "package visible")
	write(t, filepath.Join(root, ".hidden.go"), "package hidden")
	tree, err := treestampPaths(root, func(opts treestamp.Options) treestamp.Options {
		return opts.WithSkipHidden(true)
	})
	if err != nil {
		t.Fatal(err)
	}
	code, err := codewalkerPaths(root, func(w *gocodewalker.FileWalker) {
		w.IncludeHidden = false
		w.IgnoreGitModules = true
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		assertPaths(t, tree, map[string][]string{"gocodewalker": code})
		return
	}
	if hasPath(tree, ".hidden.go") || !hasPath(code, ".hidden.go") {
		t.Fatalf("unexpected Windows dot-hidden behavior: treestamp=%v gocodewalker=%v", tree, code)
	}
	t.Log("documented difference: gocodewalker v1.5.1 uses the Windows hidden attribute; Treestamp also treats dot names as hidden")
}

func TestStandardDirectorySelectionParity(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "main.go"), "package main")
	write(t, filepath.Join(root, "node_modules", "x.js"), "x")
	write(t, filepath.Join(root, "target", "out.bin"), "x")
	write(t, filepath.Join(root, "vendor", "dep.go"), "package dep")

	tree, err := treestampPaths(root, func(opts treestamp.Options) treestamp.Options {
		return opts.WithStandardSkips(true)
	})
	if err != nil {
		t.Fatal(err)
	}
	code, err := codewalkerPaths(root, func(w *gocodewalker.FileWalker) {
		w.IncludeHidden = true
		w.IgnoreGitModules = true
		w.ExcludeDirectory = []string{
			".git", ".hg", ".svn", ".venv", "__pycache__", "build",
			"coverage", "dist", "node_modules", "target", "vendor",
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, tree, map[string][]string{"gocodewalker": code})
}

func TestGitModulesSelectionParity(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".gitmodules"), `[submodule "contracts/lib/forge-std"]
	path = contracts/lib/forge-std
	url = https://github.com/foundry-rs/forge-std
`)
	write(t, filepath.Join(root, "main.go"), "package main")
	write(t, filepath.Join(root, "contracts", "keep.go"), "package keep")
	write(t, filepath.Join(root, "contracts", "lib", "forge-std", "src.go"), "package std")

	off, err := treestampPaths(root, func(opts treestamp.Options) treestamp.Options {
		return opts
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasPath(off, "contracts/lib/forge-std/src.go") {
		t.Fatalf("default Treestamp dropped a submodule file: %v", off)
	}
	tree, err := treestampPaths(root, func(opts treestamp.Options) treestamp.Options {
		return opts.WithGitModules(true)
	})
	if err != nil {
		t.Fatal(err)
	}
	code, err := codewalkerPaths(root, func(w *gocodewalker.FileWalker) {
		w.IncludeHidden = true
		w.IgnoreGitModules = false
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, tree, map[string][]string{"gocodewalker": code})
	if hasPath(tree, "contracts/lib/forge-std/src.go") || !hasPath(tree, "contracts/keep.go") {
		t.Fatalf("gitmodules selection: %v", tree)
	}
}

func TestBinarySelectionParity(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "text.txt"), "text")
	write(t, filepath.Join(root, "binary.bin"), "a\x00bc")

	opts := parityOptions().MetadataOnly()
	opts.DetectBinaryFiles = true
	report, err := treestamp.NewScanner(root, treestamp.WithOptions(opts))
	if err != nil {
		t.Fatal(err)
	}
	full, err := report.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var tree []string
	for _, file := range full.Files {
		tree = append(tree, filepath.ToSlash(file.Relative))
	}
	code, err := codewalkerPaths(root, func(w *gocodewalker.FileWalker) {
		w.IncludeHidden = true
		w.IgnoreGitModules = true
		w.IgnoreBinaryFiles = true
		w.IgnoreBinaryFileBytes = 4
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, tree, map[string][]string{"gocodewalker": code})
}

func treestampPaths(root string, configure func(treestamp.Options) treestamp.Options) ([]string, error) {
	opts := configure(parityOptions())
	scanner, err := treestamp.NewScanner(root, treestamp.WithOptions(opts))
	if err != nil {
		return nil, err
	}
	return scanner.ScanPaths(context.Background())
}

func parityOptions() treestamp.Options {
	return treestamp.DefaultOptions().
		MetadataOnly().
		WithStandardSkips(false).
		WithSkipHidden(false).
		WithIgnoreFiles(".gitignore", ".ignore")
}

func codewalkerPaths(root string, configure func(*gocodewalker.FileWalker)) ([]string, error) {
	files := make(chan *gocodewalker.File, 64)
	done := make(chan error, 1)
	walker := gocodewalker.NewFileWalker(root, files)
	configure(walker)
	go func() { done <- walker.Start() }()
	var out []string
	for file := range files {
		out = append(out, relative(root, file.Location))
	}
	return out, <-done
}

func TestRegexAndDirectoryFilterParity(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "keep.go"), "package keep")
	write(t, filepath.Join(root, "drop.md"), "md")
	write(t, filepath.Join(root, "skip.go"), "package skip")
	write(t, filepath.Join(root, "vendor", "lib.go"), "package lib")

	tree, err := treestampPaths(root, func(opts treestamp.Options) treestamp.Options {
		return opts.WithIncludeFilenameRegex(`\.go$`).WithExcludeFilenames("skip.go").WithExcludeDirectories("vendor")
	})
	if err != nil {
		t.Fatal(err)
	}
	code, err := codewalkerPaths(root, func(w *gocodewalker.FileWalker) {
		w.IncludeHidden = true
		w.IgnoreGitModules = true
		w.IncludeFilenameRegex = []*regexp.Regexp{regexp.MustCompile(`\.go$`)}
		w.ExcludeFilename = []string{"skip.go"}
		w.ExcludeDirectory = []string{"vendor"}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasPath(tree, "keep.go") || hasPath(tree, "drop.md") || hasPath(tree, "skip.go") || hasPath(tree, "vendor/lib.go") {
		t.Fatalf("treestamp filters %v", tree)
	}
	if !hasPath(code, "keep.go") || hasPath(code, "drop.md") || hasPath(code, "vendor/lib.go") {
		t.Fatalf("gocodewalker regex/dir %v", code)
	}
	if !hasPath(code, "skip.go") {
		t.Fatal("expected gocodewalker include-regex to overwrite ExcludeFilename")
	}
}

func TestFileWalkerAndRepoRootHelpers(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref")
	write(t, filepath.Join(root, "nested", "keep.go"), "package keep")
	write(t, filepath.Join(root, "nested", "drop.txt"), "x")
	if got := treestamp.FindRepositoryRoot(filepath.Join(root, "nested")); filepath.Clean(got) != filepath.Clean(root) {
		t.Fatalf("repo root %s want %s", got, root)
	}
	files := make(chan *treestamp.File, 8)
	walker := treestamp.NewFileWalker(root, files)
	walker.IncludeFilenameRegex = []*regexp.Regexp{regexp.MustCompile(`\.go$`)}
	walker.IncludeHidden(true)
	if err := walker.Start(); err != nil {
		t.Fatal(err)
	}
	var seen []string
	for file := range files {
		seen = append(seen, file.Filename)
	}
	if len(seen) != 1 || seen[0] != "keep.go" {
		t.Fatalf("filewalker %v", seen)
	}
}

func BenchmarkSelectTreestamp(b *testing.B) {
	root := makeSelectCorpus(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := treestampPaths(root, func(opts treestamp.Options) treestamp.Options {
			return opts.WithIncludeFilenameRegex(`\.go$`)
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSelectGocodewalker(b *testing.B) {
	root := makeSelectCorpus(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := codewalkerPaths(root, func(w *gocodewalker.FileWalker) {
			w.IncludeHidden = true
			w.IgnoreGitModules = true
			w.IncludeFilenameRegex = []*regexp.Regexp{regexp.MustCompile(`\.go$`)}
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func makeSelectCorpus(tb testing.TB) string {
	tb.Helper()
	root := tb.TempDir()
	for i := 0; i < 80; i++ {
		write(tb, filepath.Join(root, "src", filepath.Join(fmt.Sprintf("g%d", i), "f.go")), "package f")
		write(tb, filepath.Join(root, "src", filepath.Join(fmt.Sprintf("g%d", i), "f.md")), "md")
	}
	return root
}

func hasPath(paths []string, wanted string) bool {
	for _, path := range paths {
		if path == wanted {
			return true
		}
	}
	return false
}
