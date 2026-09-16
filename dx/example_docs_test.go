package treestamp_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing/fstest"

	"github.com/sergii-ziborov/treestamp"
)

func exampleTree(files map[string]string) string {
	dir, err := os.MkdirTemp("", "treestamp-ex-")
	if err != nil {
		panic(err)
	}
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			panic(err)
		}
	}
	return dir
}

//region example-scan-paths
func ExampleScanPaths() {
	root := exampleTree(map[string]string{
		"keep.go":    "package keep\n",
		".gitignore": "skip.bin\n",
		"skip.bin":   "hidden",
	})
	defer os.RemoveAll(root)
	paths, err := treestamp.ScanPaths(context.Background(), root)
	if err != nil {
		fmt.Println("error")
		return
	}
	fmt.Println(containsPath(paths, "keep.go"), containsPath(paths, "skip.bin"))
	// Output:
	// true false
}

//endregion

//region example-scan-with-extensions
func ExampleNewScanner_withExtensions() {
	root := exampleTree(map[string]string{
		"keep.go":  "package keep\n",
		"notes.md": "# notes\n",
	})
	defer os.RemoveAll(root)
	opts := treestamp.DefaultOptions().WithExtensions("go")
	scanner, err := treestamp.NewScanner(root, treestamp.WithOptions(opts))
	if err != nil {
		fmt.Println("config")
		return
	}
	report, err := scanner.Scan(context.Background())
	if err != nil {
		fmt.Println("error")
		return
	}
	fmt.Println(len(report.Files), report.Complete, report.Termination.String() == "")
	// Output:
	// 1 true true
}

//endregion

//region example-explain
func ExampleScanner_Explain() {
	root := exampleTree(map[string]string{
		".gitignore":         "# keep\ngenerated/**\n",
		"generated/model.go": "package g\n",
		"keep.go":            "package keep\n",
	})
	defer os.RemoveAll(root)
	scanner, err := treestamp.NewScanner(root)
	if err != nil {
		fmt.Println("config")
		return
	}
	why, err := scanner.Explain("generated/model.go")
	if err != nil {
		fmt.Println("error")
		return
	}
	fmt.Println(why.Outcome, why.Reason, why.Source, why.Line, why.Pattern)
	// Output:
	// excluded ignore_rule .gitignore 2 generated/**
}

//endregion

//region example-cache
func ExampleScanReport_ToCache() {
	root := exampleTree(map[string]string{"a.txt": "abc"})
	defer os.RemoveAll(root)
	first, err := treestamp.Scan(context.Background(), root)
	if err != nil {
		fmt.Println("error")
		return
	}
	cache := first.ToCache()
	scanner, err := treestamp.NewScanner(root)
	if err != nil {
		fmt.Println("config")
		return
	}
	second, err := scanner.ScanCached(context.Background(), &cache)
	if err != nil {
		fmt.Println("error")
		return
	}
	fmt.Println(first.Revision == second.Revision, second.Cache.ReusedHashes > 0)
	// Output:
	// true true
}

//endregion

//region example-read-bounded
func ExampleSnapshotContentProvider_ReadBounded() {
	root := exampleTree(map[string]string{"a.txt": "abcdef"})
	defer os.RemoveAll(root)
	report, err := treestamp.Scan(context.Background(), root)
	if err != nil {
		fmt.Println("error")
		return
	}
	provider, err := report.ContentProvider()
	if err != nil {
		fmt.Println("provider")
		return
	}
	if _, err := provider.ReadBounded("a.txt", 2); err == nil {
		fmt.Println("expected limit")
		return
	}
	got, err := provider.ReadBounded("a.txt", 64)
	if err != nil {
		fmt.Println("read")
		return
	}
	fmt.Println(string(got.Bytes), got.Evidence == treestamp.SnapshotSHA256)
	// Output:
	// abcdef true
}

//endregion

//region example-walk-fs
func ExampleWalkFS() {
	fsys := fstest.MapFS{
		"a.txt":     {Data: []byte("a")},
		"sub/b.txt": {Data: []byte("b")},
	}
	var names []string
	err := treestamp.WalkFS(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		names = append(names, path)
		return nil
	})
	if err != nil {
		fmt.Println("error")
		return
	}
	fmt.Println(strings.Join(names, ","))
	// Output:
	// a.txt,sub/b.txt
}

//endregion

//region example-tree-snapshot
func ExampleTreeSnapshot_Apply() {
	first := []treestamp.ScannedFile{
		{Relative: "a.go", ContentHash: "ha", Bytes: 1},
		{Relative: "b.go", ContentHash: "hb", Bytes: 2},
	}
	snap := treestamp.SnapshotFromFiles(first)
	next := snap.Apply([]treestamp.ScannedFile{{Relative: "b.go", ContentHash: "hb2", Bytes: 3}}, nil)
	fmt.Println(strings.HasPrefix(snap.TreeRevision(), "tree2:"))
	fmt.Println(snap.TreeRevision() == next.TreeRevision())
	// Output:
	// true
	// false
}

//endregion

//region example-portable
func ExampleScanReport_ToPortable() {
	root := exampleTree(map[string]string{"a.txt": "abc"})
	defer os.RemoveAll(root)
	report, err := treestamp.Scan(context.Background(), root)
	if err != nil {
		fmt.Println("error")
		return
	}
	portable := report.ToPortable()
	fmt.Println(portable.Files[0].Relative, report.Root != "")
	// Output:
	// a.txt true
}

//endregion

//region example-invalid-regex
func ExampleNewScanner_invalidRegex() {
	opts := treestamp.DefaultOptions().WithIncludeFilenameRegex("[")
	_, err := treestamp.NewScanner(".", treestamp.WithOptions(opts))
	var typed *treestamp.Error
	if errors.As(err, &typed) {
		fmt.Println(typed.Code)
		return
	}
	fmt.Println("other")
	// Output:
	// invalid
}

//endregion

//region example-scan-into-stop
func ExampleScanner_ScanInto_stop() {
	root := exampleTree(map[string]string{"a.txt": "a", "b.txt": "b"})
	defer os.RemoveAll(root)
	scanner, err := treestamp.NewScanner(root)
	if err != nil {
		fmt.Println("config")
		return
	}
	stream, err := scanner.ScanInto(context.Background(), treestamp.ScanSinkFunc(func(*treestamp.ScannedFile) treestamp.ScanSinkControl {
		return treestamp.ScanSinkStop
	}))
	if err != nil {
		fmt.Println("error")
		return
	}
	fmt.Println(stream.Stopped, stream.Emitted)
	// Output:
	// true 1
}

//endregion

//region example-scan-with
func ExampleScanWith() {
	root := exampleTree(map[string]string{"keep.go": "package keep\n", "skip_test.go": "package keep\n"})
	defer os.RemoveAll(root)
	report, err := treestamp.ScanWith(context.Background(), root,
		treestamp.WithExtensions("go"),
		treestamp.WithExcludeGlobs("*_test.go"),
	)
	if err != nil {
		fmt.Println("error")
		return
	}
	fmt.Println(len(report.Files), report.Files[0].Relative)
	// Output:
	// 1 keep.go
}

//endregion

//region example-each-file
func ExampleEachFile() {
	root := exampleTree(map[string]string{"a.go": "package a\n"})
	defer os.RemoveAll(root)
	_, err := treestamp.EachFile(context.Background(), root, func(file treestamp.ScannedFile, data []byte) error {
		fmt.Println(file.Relative, strings.Contains(string(data), "package a"))
		return nil
	}, treestamp.WithExtensions("go"))
	if err != nil {
		fmt.Println("error")
	}
	// Output:
	// a.go true
}

//endregion

//region example-compile-plan
func ExampleCompile() {
	root := exampleTree(map[string]string{"keep.go": "package keep\n"})
	defer os.RemoveAll(root)
	plan, err := treestamp.Compile(treestamp.WithExtensions("go"))
	if err != nil {
		fmt.Println("config")
		return
	}
	report, err := plan.Scan(context.Background(), root)
	if err != nil {
		fmt.Println("error")
		return
	}
	fmt.Println(strings.Contains(plan.Describe(), "profile=repository"), len(report.Files))
	// Output:
	// true 1
}

//endregion

func containsPath(paths []string, name string) bool {
	for _, path := range paths {
		if path == name || strings.HasSuffix(path, "/"+name) {
			return true
		}
	}
	return false
}
