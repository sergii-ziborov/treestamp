package compat_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/charlievieth/fastwalk"
	"github.com/karrick/godirwalk"
	"github.com/sergii-ziborov/treestamp"
)

func TestSymlinksNotFollowedParity(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "target", "file.txt"), "x")
	write(t, filepath.Join(root, "plain.txt"), "plain")
	mustSymlink(t, filepath.Join(root, "target"), filepath.Join(root, "dir-link"))
	mustSymlink(t, filepath.Join(root, "plain.txt"), filepath.Join(root, "file-link"))
	mustSymlink(t, filepath.Join(root, "missing"), filepath.Join(root, "dangling"))

	tree, treeErr := collectTreestamp(root, "", false)
	fast, fastErr := collectFastwalk(root, "", false)
	godir, godirErr := collectGodirwalk(root, "", false, false)
	requireWalkErrors(t, treeErr, fastErr, godirErr)
	assertNodeSet(t, tree, map[string][]node{"fastwalk": fast, "godirwalk": godir})
	if hasNode(tree, "dir-link/file.txt") {
		t.Fatal("a no-follow walk traversed a directory link")
	}
}

func TestFollowRejectsRootEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	write(t, filepath.Join(outside, "outside.txt"), "outside")
	mustSymlink(t, outside, filepath.Join(root, "escape"))

	tree, treeErr := collectTreestamp(root, "", true)
	fast, fastErr := collectFastwalk(root, "", true)
	godir, godirErr := collectGodirwalk(root, "", true, false)
	requireWalkErrors(t, treeErr, fastErr, godirErr)
	if hasNode(tree, "escape/outside.txt") {
		t.Fatal("Treestamp followed a directory link outside its root")
	}
	if !hasNode(fast, "escape/outside.txt") || !hasNode(godir, "escape/outside.txt") {
		t.Fatalf("competitor escape behavior changed: fast=%v godir=%v", fast, godir)
	}
	reasons, err := treestampSkipReasons(root)
	if err != nil || reasons["escape"] != treestamp.WalkSkipPathEscape {
		t.Fatalf("path escape evidence: %v %v", reasons, err)
	}
	t.Log("documented difference: fastwalk and godirwalk follow directory symlinks outside the walk root; Treestamp records path_escape")
}

func TestFollowStopsAncestorLoop(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "dir", "file.txt"), "x")
	mustSymlink(t, root, filepath.Join(root, "dir", "back"))

	tree, treeErr := collectTreestamp(root, "", true)
	fast, fastErr := collectFastwalk(root, "", true)
	if err := errors.Join(treeErr, fastErr); err != nil {
		t.Fatal(err)
	}
	if hasNode(tree, "dir/back/dir") || hasNode(fast, "dir/back/dir") {
		t.Fatalf("guarded walker recursed into loop: treestamp=%v fastwalk=%v", tree, fast)
	}
	reasons, err := treestampSkipReasons(root)
	if err != nil || reasons["dir/back"] != treestamp.WalkSkipSymlinkLoop {
		t.Fatalf("loop evidence: %v %v", reasons, err)
	}
	assertGodirwalkHasNoLoopGuard(t, root)
	t.Log("documented difference: godirwalk has no ancestor-loop guard; Treestamp records symlink_loop and fastwalk stops recursion")
}

func TestMaxDepthPrecedesLoop(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "dir", "file.txt"), "x")
	mustSymlink(t, root, filepath.Join(root, "dir", "back"))
	maxDepth := 2
	opts := treestamp.DefaultWalkOptions()
	opts.FollowLinks = true
	opts.MaxDepth = &maxDepth
	reasons, err := collectTreestampSkipReasons(root, opts)
	if err != nil || reasons["dir/back"] != treestamp.WalkSkipMaxDepth {
		t.Fatalf("skip precedence: %v %v", reasons, err)
	}
}

func TestSelectiveMaxDepthPrecedesLoop(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "dir", "file.txt"), "x")
	mustSymlink(t, root, filepath.Join(root, "dir", "back"))
	maxDepth := 2
	opts := treestamp.DefaultWalkOptions()
	opts.MaxDepth = &maxDepth
	walker, err := treestamp.NewWalkerWithOptions(root, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	for {
		entry, nextErr := walker.Next()
		if nextErr != nil {
			t.Fatal(nextErr)
		}
		if relative(root, entry.Path()) != "dir/back" {
			continue
		}
		if err := walker.TraverseCurrentSymlink(); err != nil {
			t.Fatal(err)
		}
		if entry.SkipReason() != treestamp.WalkSkipMaxDepth {
			t.Fatalf("skip reason = %v", entry.SkipReason())
		}
		return
	}
}

func TestDanglingFollowErrorDifference(t *testing.T) {
	root := t.TempDir()
	mustSymlink(t, filepath.Join(root, "missing"), filepath.Join(root, "dangling"))
	if _, err := collectTreestamp(root, "", true); err == nil {
		t.Fatal("Treestamp did not report dangling-link metadata failure")
	}
	if _, err := collectFastwalk(root, "", true); err != nil {
		t.Fatalf("fastwalk unexpectedly reports dangling target: %v", err)
	}
	if _, err := collectGodirwalk(root, "", true, false); err == nil {
		t.Fatal("godirwalk did not report dangling-link metadata failure")
	}
	t.Log("documented difference: fastwalk silently ignores a dangling target; Treestamp and godirwalk report it")
}

func TestRootSymlinkPolicies(t *testing.T) {
	target := t.TempDir()
	write(t, filepath.Join(target, "file.txt"), "x")
	parent := t.TempDir()
	root := filepath.Join(parent, "root-link")
	mustSymlink(t, target, root)

	tree, treeErr := collectTreestamp(root, "", false)
	fast, fastErr := collectFastwalk(root, "", false)
	if err := errors.Join(treeErr, fastErr); err != nil {
		t.Fatal(err)
	}
	if !equalStrings(nodePaths(tree), nodePaths(fast)) || !hasNode(tree, "file.txt") {
		t.Fatal("default root-symlink policy did not follow the root")
	}
	opts := treestamp.DefaultWalkOptions()
	opts.RootSymlinkPolicy = treestamp.RootReject
	if _, err := treestamp.NewParallelWalker(root).Options(opts).Walk(); err == nil {
		t.Fatal("RootReject accepted a symlink root")
	}
	if _, err := collectGodirwalk(root, "", false, false); err == nil {
		t.Fatal("godirwalk unexpectedly accepted a no-follow symlink root")
	}
	t.Log("documented difference: Treestamp preserves the root link type while following it; fastwalk reports a directory and godirwalk rejects it")
}

func TestSelectiveDirectorySymlinkFollowParity(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "chosen-target", "chosen.txt"), "chosen")
	write(t, filepath.Join(root, "ignored-target", "ignored.txt"), "ignored")
	mustSymlink(t, filepath.Join(root, "chosen-target"), filepath.Join(root, "chosen"))
	mustSymlink(t, filepath.Join(root, "ignored-target"), filepath.Join(root, "ignored"))

	fast, err := collectFastwalkSelective(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, workers := range []int{0, 2} {
		t.Run(workerLabel(workers), func(t *testing.T) {
			tree, treeErr := collectTreestampSelective(root, workers)
			if treeErr != nil {
				t.Fatal(treeErr)
			}
			assertNodeSet(t, fast, map[string][]node{"treestamp": tree})
			if !hasNode(tree, "chosen/chosen.txt") || hasNode(tree, "ignored/ignored.txt") {
				t.Fatalf("selective traversal failed: %v", nodePaths(tree))
			}
		})
	}
}

func TestWalkParallelSelectiveFollow(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "target", "file.txt"), "x")
	mustSymlink(t, filepath.Join(root, "target"), filepath.Join(root, "chosen"))
	var mu sync.Mutex
	var paths []string
	err := treestamp.WalkParallel(root, 2, func(entry *treestamp.WalkEntry) error {
		mu.Lock()
		paths = append(paths, relative(root, entry.Path()))
		mu.Unlock()
		if entry.IsSymlink() {
			return treestamp.ErrTraverseLink
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(paths, "chosen/file.txt") {
		t.Fatalf("selected directory was not traversed: %v", paths)
	}
}

func TestParallelVisitSelectiveFollow(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "target", "file.txt"), "x")
	mustSymlink(t, filepath.Join(root, "target"), filepath.Join(root, "chosen"))
	var mu sync.Mutex
	var paths []string
	_, err := treestamp.NewParallelWalker(root).WithParallelism(2).Visit(func(ev treestamp.WalkEvent) treestamp.WalkControl {
		if ev.Err != nil {
			t.Errorf("visit error: %v", ev.Err)
			return treestamp.WalkQuit
		}
		mu.Lock()
		paths = append(paths, filepath.ToSlash(ev.Entry.RelativePath()))
		mu.Unlock()
		if ev.Entry.IsSymlink() {
			return treestamp.WalkTraverseLink
		}
		return treestamp.WalkContinue
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(paths, "chosen/file.txt") {
		t.Fatalf("selected directory was not traversed: %v", paths)
	}
}

func TestSelectiveFollowKeepsSafetyGuards(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	write(t, filepath.Join(root, "dir", "inside.txt"), "inside")
	write(t, filepath.Join(outside, "outside.txt"), "outside")
	mustSymlink(t, root, filepath.Join(root, "dir", "back"))
	mustSymlink(t, outside, filepath.Join(root, "escape"))

	for _, workers := range []int{0, 2} {
		t.Run(workerLabel(workers), func(t *testing.T) {
			paths, err := collectAllSelectedLinks(root, workers)
			if err != nil {
				t.Fatal(err)
			}
			if hasNode(paths, "escape/outside.txt") || hasNode(paths, "dir/back/dir") {
				t.Fatalf("selective follow bypassed a safety guard: %v", nodePaths(paths))
			}
		})
	}
}

func TestPullWalkerSelectiveFollowEvidence(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	write(t, filepath.Join(root, "target", "file.txt"), "inside")
	write(t, filepath.Join(outside, "outside.txt"), "outside")
	mustSymlink(t, filepath.Join(root, "target"), filepath.Join(root, "chosen"))
	mustSymlink(t, root, filepath.Join(root, "target", "back"))
	mustSymlink(t, outside, filepath.Join(root, "escape"))

	paths, reasons := collectPullSelected(t, root)
	if !hasNode(paths, "chosen/file.txt") {
		t.Fatalf("selected directory was not traversed: %v", nodePaths(paths))
	}
	if reasons["escape"] != treestamp.WalkSkipPathEscape {
		t.Fatalf("escape reason = %v", reasons["escape"])
	}
	if reasons["target/back"] != treestamp.WalkSkipSymlinkLoop {
		t.Fatalf("loop reason = %v", reasons["target/back"])
	}
}

func TestSelectiveFollowRejectsInvalidTargets(t *testing.T) {
	for _, target := range []string{"file", "missing"} {
		t.Run(target, func(t *testing.T) {
			root := t.TempDir()
			linkTarget := filepath.Join(root, target)
			if target == "file" {
				write(t, linkTarget, "x")
			}
			mustSymlink(t, linkTarget, filepath.Join(root, "link"))
			for _, workers := range []int{0, 2} {
				err := walkAllSelectedLinks(root, workers, nil)
				if err == nil {
					t.Fatalf("%s target accepted with %d workers", target, workers)
				}
			}
		})
	}
}

func treestampSkipReasons(root string) (map[string]treestamp.WalkSkipReason, error) {
	opts := treestamp.DefaultWalkOptions()
	opts.FollowLinks = true
	return collectTreestampSkipReasons(root, opts)
}

func collectTreestampSkipReasons(root string, opts treestamp.WalkOptions) (map[string]treestamp.WalkSkipReason, error) {
	reasons := map[string]treestamp.WalkSkipReason{}
	var mu sync.Mutex
	_, err := treestamp.NewParallelWalker(root).Options(opts).WithParallelism(2).Visit(func(ev treestamp.WalkEvent) treestamp.WalkControl {
		if ev.Err != nil {
			return treestamp.WalkContinue
		}
		mu.Lock()
		reasons[filepath.ToSlash(ev.Entry.RelativePath())] = ev.Entry.SkipReason()
		mu.Unlock()
		return treestamp.WalkContinue
	})
	return reasons, err
}

func assertGodirwalkHasNoLoopGuard(t *testing.T, root string) {
	t.Helper()
	stop := errors.New("loop depth reached")
	err := godirwalk.Walk(root, &godirwalk.Options{
		FollowSymbolicLinks: true,
		Callback: func(path string, _ *godirwalk.Dirent) error {
			if strings.Count(relative(root, path), "back") >= 2 {
				return stop
			}
			return nil
		},
	})
	if !errors.Is(err, stop) {
		t.Fatalf("godirwalk unexpectedly guarded the loop: %v", err)
	}
}

func collectTreestampSelective(root string, workers int) ([]node, error) {
	var mu sync.Mutex
	var out []node
	err := treestamp.WalkWithConfig(root, treestamp.Config{NumWorkers: workers}, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		mu.Lock()
		out = append(out, makeNode(root, path, entry))
		mu.Unlock()
		if relative(root, path) == "chosen" {
			target, err := treestamp.StatDirEntry(path, entry)
			if err != nil {
				return err
			}
			if target.IsDir() {
				return treestamp.ErrTraverseLink
			}
		}
		return nil
	})
	return out, err
}

func collectFastwalkSelective(root string) ([]node, error) {
	var mu sync.Mutex
	var out []node
	err := fastwalk.Walk(nil, root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		mu.Lock()
		out = append(out, makeNode(root, path, entry))
		mu.Unlock()
		if relative(root, path) == "chosen" {
			target, err := fastwalk.StatDirEntry(path, entry)
			if err != nil {
				return err
			}
			if target.IsDir() {
				return fastwalk.ErrTraverseLink
			}
		}
		return nil
	})
	return out, err
}

func collectAllSelectedLinks(root string, workers int) ([]node, error) {
	var out []node
	err := walkAllSelectedLinks(root, workers, &out)
	return out, err
}

func walkAllSelectedLinks(root string, workers int, out *[]node) error {
	var mu sync.Mutex
	return treestamp.WalkWithConfig(root, treestamp.Config{NumWorkers: workers}, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if out != nil {
			mu.Lock()
			*out = append(*out, makeNode(root, path, entry))
			mu.Unlock()
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return treestamp.ErrTraverseLink
		}
		return nil
	})
}

func collectPullSelected(t *testing.T, root string) ([]node, map[string]treestamp.WalkSkipReason) {
	t.Helper()
	walker, err := treestamp.NewWalker(root)
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	var paths []node
	reasons := map[string]treestamp.WalkSkipReason{}
	for {
		entry, nextErr := walker.Next()
		if errors.Is(nextErr, io.EOF) {
			return paths, reasons
		}
		if nextErr != nil {
			t.Fatal(nextErr)
		}
		info, statErr := os.Lstat(entry.Path())
		if statErr != nil {
			t.Fatal(statErr)
		}
		paths = append(paths, makeNode(root, entry.Path(), fs.FileInfoToDirEntry(info)))
		if entry.IsSymlink() {
			if err := walker.TraverseCurrentSymlink(); err != nil {
				t.Fatal(err)
			}
			reasons[relative(root, entry.Path())] = entry.SkipReason()
		}
	}
}

func workerLabel(workers int) string {
	if workers == 0 {
		return "serial"
	}
	return "parallel"
}

func mustSymlink(tb testing.TB, target, link string) {
	tb.Helper()
	if err := os.Symlink(target, link); err != nil {
		tb.Skipf("symlinks unavailable: %v", err)
	}
}

func BenchmarkSelectiveFollowTreestampSerial(b *testing.B) {
	benchSelectiveFollow(b, func(root string, fn fs.WalkDirFunc) error {
		return treestamp.WalkWithConfig(root, treestamp.Config{}, fn)
	}, treestamp.StatDirEntry, treestamp.ErrTraverseLink)
}

func BenchmarkSelectiveFollowTreestampParallel(b *testing.B) {
	benchSelectiveFollow(b, func(root string, fn fs.WalkDirFunc) error {
		return treestamp.WalkWithConfig(root, treestamp.Config{NumWorkers: 2}, fn)
	}, treestamp.StatDirEntry, treestamp.ErrTraverseLink)
}

func BenchmarkSelectiveFollowFastwalk(b *testing.B) {
	benchSelectiveFollow(b, func(root string, fn fs.WalkDirFunc) error {
		return fastwalk.Walk(&fastwalk.Config{NumWorkers: 2}, root, fn)
	}, fastwalk.StatDirEntry, fastwalk.ErrTraverseLink)
}

func benchSelectiveFollow(
	b *testing.B,
	walk func(string, fs.WalkDirFunc) error,
	stat func(string, fs.DirEntry) (fs.FileInfo, error),
	traverse error,
) {
	root := makeSelectiveCorpus(b, 32)
	visit := func(path string, entry fs.DirEntry, _ error) error {
		if filepath.Base(path) != "chosen" || entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		target, err := stat(path, entry)
		if err != nil {
			return err
		}
		if target.IsDir() {
			return traverse
		}
		return nil
	}
	benchRepeat(b, func() (int, error) { return countWalk(walk, root, visit) })
}

func hasNode(nodes []node, relative string) bool {
	for _, item := range nodes {
		if item.Relative == relative {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func nodePaths(nodes []node) []string {
	out := make([]string, 0, len(nodes))
	for _, item := range nodes {
		out = append(out, item.Relative)
	}
	return out
}
