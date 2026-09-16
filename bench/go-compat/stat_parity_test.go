package compat_test

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/charlievieth/fastwalk"
	"github.com/sergii-ziborov/treestamp"
)

type cachedStatCase struct {
	name  string
	walk  func(string, fs.WalkDirFunc) error
	stat  func(string, fs.DirEntry) (fs.FileInfo, error)
	depth func(fs.DirEntry) int
}

func TestCachedCallbackStatRegularParity(t *testing.T) {
	for _, test := range cachedStatCases() {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			file := filepath.Join(root, "file.txt")
			write(t, file, "cached")
			err := test.walk(root, func(path string, entry fs.DirEntry, err error) error {
				if err != nil || path != file {
					return err
				}
				return checkCachedSuccess(test, path, entry, func() error { return os.Remove(file) })
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCachedCallbackTargetStatParity(t *testing.T) {
	for _, test := range cachedStatCases() {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "root")
			target := filepath.Join(base, "target.txt")
			link := filepath.Join(root, "link")
			write(t, target, "target")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			mustSymlink(t, target, link)
			err := test.walk(root, func(path string, entry fs.DirEntry, err error) error {
				if err != nil || path != link {
					return err
				}
				return checkCachedSuccess(test, path, entry, func() error { return os.Remove(target) })
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCachedCallbackTargetErrorParity(t *testing.T) {
	for _, test := range cachedStatCases() {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "missing")
			link := filepath.Join(root, "link")
			mustSymlink(t, target, link)
			err := test.walk(root, func(path string, entry fs.DirEntry, err error) error {
				if err != nil || path != link {
					return err
				}
				if _, firstErr := test.stat(path, entry); firstErr == nil {
					return fmt.Errorf("first target Stat unexpectedly succeeded")
				}
				if err := os.WriteFile(target, []byte("created"), 0o644); err != nil {
					return err
				}
				if _, secondErr := test.stat(path, entry); secondErr == nil {
					return fmt.Errorf("target Stat error was not cached")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestWalkEntryTargetStatCache(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	target := filepath.Join(base, "target.txt")
	link := filepath.Join(root, "link")
	write(t, target, "target")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, target, link)
	walker, err := treestamp.NewWalker(root)
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	for {
		entry, nextErr := walker.Next()
		if nextErr == io.EOF {
			t.Fatal("link entry not found")
		}
		if nextErr != nil {
			t.Fatal(nextErr)
		}
		if entry.Path() != link {
			continue
		}
		first, err := entry.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(target); err != nil {
			t.Fatal(err)
		}
		second, err := entry.Stat()
		if err != nil || !sameInfo(first, second) {
			t.Fatalf("WalkEntry Stat was not cached: %v", err)
		}
		return
	}
}

func TestStatDirEntryFallbacks(t *testing.T) {
	if _, err := treestamp.StatDirEntry("missing", nil); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("nil entry error = %v", err)
	}
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	write(t, file, "x")
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	entry := fs.FileInfoToDirEntry(info)
	if treestamp.DirEntryDepth(entry) != -1 {
		t.Fatal("plain DirEntry unexpectedly has a walk depth")
	}
	if got, err := treestamp.StatDirEntry(file, entry); err != nil || !sameInfo(got, info) {
		t.Fatalf("fallback Stat = %v, %v", got, err)
	}
}

func checkCachedSuccess(
	test cachedStatCase, path string, entry fs.DirEntry, mutate func() error,
) error {
	if _, ok := entry.(interface{ Stat() (fs.FileInfo, error) }); !ok {
		return fmt.Errorf("%T does not expose Stat", entry)
	}
	if test.depth(entry) != 1 {
		return fmt.Errorf("depth = %d", test.depth(entry))
	}
	first, err := test.stat(path, entry)
	if err != nil {
		return err
	}
	if err := mutate(); err != nil {
		return err
	}
	second, err := test.stat(path, entry)
	if err != nil {
		return fmt.Errorf("second Stat was not cached: %w", err)
	}
	if !sameInfo(first, second) {
		return fmt.Errorf("cached metadata changed: first=%v second=%v", first, second)
	}
	return nil
}

func sameInfo(left, right fs.FileInfo) bool {
	return left.Name() == right.Name() &&
		left.Size() == right.Size() &&
		left.Mode() == right.Mode() &&
		left.ModTime() == right.ModTime()
}

func BenchmarkCachedStatTreestampSerial(b *testing.B) {
	benchCachedStat(b, func(root string, fn fs.WalkDirFunc) error {
		return treestamp.WalkWithConfig(root, treestamp.Config{}, fn)
	}, treestamp.StatDirEntry, true)
}

func BenchmarkCachedStatTreestampParallel(b *testing.B) {
	benchCachedStat(b, func(root string, fn fs.WalkDirFunc) error {
		return treestamp.WalkWithConfig(root, treestamp.Config{NumWorkers: 2}, fn)
	}, treestamp.StatDirEntry, true)
}

func BenchmarkCachedStatFastwalk(b *testing.B) {
	benchCachedStat(b, func(root string, fn fs.WalkDirFunc) error {
		return fastwalk.Walk(&fastwalk.Config{NumWorkers: 2}, root, fn)
	}, fastwalk.StatDirEntry, true)
}

func BenchmarkCachedStatOsStat(b *testing.B) {
	benchCachedStat(b, func(root string, fn fs.WalkDirFunc) error {
		return treestamp.WalkWithConfig(root, treestamp.Config{}, fn)
	}, func(path string, _ fs.DirEntry) (fs.FileInfo, error) {
		return os.Stat(path)
	}, true)
}

func BenchmarkCachedStatRegularTreestamp(b *testing.B) {
	benchCachedStat(b, func(root string, fn fs.WalkDirFunc) error {
		return treestamp.WalkWithConfig(root, treestamp.Config{}, fn)
	}, treestamp.StatDirEntry, false)
}

func BenchmarkCachedStatRegularTreestampParallel(b *testing.B) {
	benchCachedStat(b, func(root string, fn fs.WalkDirFunc) error {
		return treestamp.WalkWithConfig(root, treestamp.Config{NumWorkers: 2}, fn)
	}, treestamp.StatDirEntry, false)
}

func BenchmarkCachedStatRegularFastwalk(b *testing.B) {
	benchCachedStat(b, func(root string, fn fs.WalkDirFunc) error {
		return fastwalk.Walk(&fastwalk.Config{NumWorkers: 2}, root, fn)
	}, fastwalk.StatDirEntry, false)
}

func BenchmarkCachedStatRegularFastwalkSerial(b *testing.B) {
	benchCachedStat(b, func(root string, fn fs.WalkDirFunc) error {
		return fastwalk.Walk(&fastwalk.Config{NumWorkers: 1}, root, fn)
	}, fastwalk.StatDirEntry, false)
}

func BenchmarkCachedStatRegularOsStat(b *testing.B) {
	benchCachedStat(b, func(root string, fn fs.WalkDirFunc) error {
		return treestamp.WalkWithConfig(root, treestamp.Config{}, fn)
	}, func(path string, _ fs.DirEntry) (fs.FileInfo, error) {
		return os.Stat(path)
	}, false)
}

func benchCachedStat(
	b *testing.B,
	walk func(string, fs.WalkDirFunc) error,
	stat func(string, fs.DirEntry) (fs.FileInfo, error),
	withLinks bool,
) {
	links := 0
	if withLinks {
		links = 200
	}
	root := makeStatCorpus(b, 400, links)
	visit := func(path string, entry fs.DirEntry, _ error) error {
		if _, err := stat(path, entry); err != nil {
			return err
		}
		if _, err := stat(path, entry); err != nil {
			return err
		}
		return nil
	}
	benchRepeat(b, func() (int, error) { return countWalk(walk, root, visit) })
}

func cachedStatCases() []cachedStatCase {
	return []cachedStatCase{
		{
			name: "treestamp serial",
			walk: func(root string, fn fs.WalkDirFunc) error {
				return treestamp.WalkWithConfig(root, treestamp.Config{}, fn)
			},
			stat: treestamp.StatDirEntry, depth: treestamp.DirEntryDepth,
		},
		{
			name: "treestamp parallel",
			walk: func(root string, fn fs.WalkDirFunc) error {
				return treestamp.WalkWithConfig(root, treestamp.Config{NumWorkers: 2}, fn)
			},
			stat: treestamp.StatDirEntry, depth: treestamp.DirEntryDepth,
		},
		{
			name: "fastwalk",
			walk: func(root string, fn fs.WalkDirFunc) error {
				return fastwalk.Walk(&fastwalk.Config{NumWorkers: 2}, root, fn)
			},
			stat: fastwalk.StatDirEntry, depth: fastwalk.DirEntryDepth,
		},
	}
}
