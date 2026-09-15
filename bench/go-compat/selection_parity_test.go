package compat_test

import (
	"context"
	"path/filepath"
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

func hasPath(paths []string, wanted string) bool {
	for _, path := range paths {
		if path == wanted {
			return true
		}
	}
	return false
}
