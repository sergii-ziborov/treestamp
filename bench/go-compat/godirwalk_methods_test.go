package compat_test

import (
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/karrick/godirwalk"
	"github.com/sergii-ziborov/treestamp"
	tsgodirwalk "github.com/sergii-ziborov/treestamp/compat/godirwalk"
)

func TestGodirwalkAllMethodsParity(t *testing.T) {
	root := makeTree(t)
	scratch := make([]byte, sharedScratchSize())
	want, err := sortedGodirwalk(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := walkDirsRelatives(root, false)
	if err != nil || !equalStrings(want, got) {
		t.Fatalf("WalkDirs order\nwant %#v\n got %#v err=%v", want, got, err)
	}
	alias, err := walkCompatRelatives(root)
	if err != nil || !equalStrings(want, alias) {
		t.Fatalf("compat Walk\nwant %#v\n got %#v err=%v", want, alias, err)
	}
	assertDirentMethods(t, root, scratch)
	assertSkipThisParity(t, root)
	assertPostChildrenParity(t, root)
	assertScannerParity(t, root, scratch)
	assertAllowNonDirectoryParity(t, filepath.Join(root, "a.txt"))
	assertErrorCallbackParity(t, root)
	assertScratchFloor(t)
	t.Run("follow", func(t *testing.T) { assertFollowParity(t, root) })
}

func walkDirsRelatives(root string, unsorted bool) ([]string, error) {
	var out []string
	err := treestamp.WalkDirs(root, treestamp.DirWalkOptions{
		Unsorted: unsorted,
		Callback: func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			out = append(out, relative(root, path))
			return nil
		},
	})
	return out, err
}

func walkCompatRelatives(root string) ([]string, error) {
	var out []string
	err := tsgodirwalk.Walk(root, &tsgodirwalk.Options{
		Callback: func(path string, _ *tsgodirwalk.Dirent) error {
			out = append(out, relative(root, path))
			return nil
		},
	})
	return out, err
}

func assertDirentMethods(t *testing.T, root string, scratch []byte) {
	t.Helper()
	want, err := godirwalk.ReadDirents(root, scratch)
	if err != nil {
		t.Fatal(err)
	}
	got, err := treestamp.ReadDirentsScratch(root, scratch)
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(godirwalkDirentKeys(want), treestampDirentKeys(got)) {
		t.Fatalf("ReadDirents\nwant %#v\n got %#v", godirwalkDirentKeys(want), treestampDirentKeys(got))
	}
	alias, err := tsgodirwalk.ReadDirents(root, scratch)
	if err != nil || !equalStrings(godirwalkDirentKeys(want), compatDirentKeys(alias)) {
		t.Fatalf("compat ReadDirents %#v err=%v", alias, err)
	}
	wantNames, err := godirwalk.ReadDirnames(root, scratch)
	if err != nil {
		t.Fatal(err)
	}
	gotNames, err := treestamp.ReadDirnames(root, scratch)
	if err != nil {
		t.Fatal(err)
	}
	aliasNames, err := tsgodirwalk.ReadDirnames(root, scratch)
	sort.Strings(wantNames)
	sort.Strings(gotNames)
	sort.Strings(aliasNames)
	if err != nil || !equalStrings(wantNames, gotNames) || !equalStrings(wantNames, aliasNames) {
		t.Fatalf("ReadDirnames %v %v %v err=%v", wantNames, gotNames, aliasNames, err)
	}
	assertNewDirentFlags(t, filepath.Join(root, "a.txt"), root)
}

func assertNewDirentFlags(t *testing.T, file, dir string) {
	t.Helper()
	karrick, err := godirwalk.NewDirent(file)
	if err != nil {
		t.Fatal(err)
	}
	ours, err := tsgodirwalk.NewDirent(file)
	if err != nil || !sameDirentFlags(ours, karrick) {
		t.Fatalf("NewDirent file ours=%+v want=%+v err=%v", ours, karrick, err)
	}
	kdir, err := godirwalk.NewDirent(dir)
	if err != nil {
		t.Fatal(err)
	}
	odir, err := tsgodirwalk.NewDirent(dir)
	if err != nil || !sameDirentFlags(odir, kdir) {
		t.Fatalf("NewDirent dir ours=%+v want=%+v err=%v", odir, kdir, err)
	}
}

func sameDirentFlags(ours *tsgodirwalk.Dirent, want *godirwalk.Dirent) bool {
	return ours != nil && want != nil && ours.Name() == want.Name() &&
		ours.IsRegular() == want.IsRegular() && ours.IsDir() == want.IsDir() &&
		ours.IsSymlink() == want.IsSymlink() && ours.IsDevice() == want.IsDevice() &&
		ours.ModeType() == want.ModeType()
}

func compatDirentKeys(ents tsgodirwalk.Dirents) []string {
	out := make([]string, 0, len(ents))
	for _, ent := range ents {
		out = append(out, ent.Name()+":"+ent.ModeType().String())
	}
	sort.Strings(out)
	return out
}

func assertSkipThisParity(t *testing.T, root string) {
	t.Helper()
	skip := func(name string) bool { return name == "z.txt" || name == "skip" }
	want, err := collectGodirwalkSkipThis(root, skip)
	if err != nil {
		t.Fatal(err)
	}
	got, err := collectWalkDirsSkipThis(root, skip)
	if err != nil || !equalStrings(want, got) {
		t.Fatalf("WalkDirs SkipThis\nwant %#v\n got %#v err=%v", want, got, err)
	}
	alias, err := collectCompatSkipThis(root, skip)
	if err != nil || !equalStrings(want, alias) {
		t.Fatalf("compat SkipThis\nwant %#v\n got %#v err=%v", want, alias, err)
	}
}

func collectGodirwalkSkipThis(root string, skip func(string) bool) ([]string, error) {
	var out []string
	err := godirwalk.Walk(root, &godirwalk.Options{
		Callback: func(path string, de *godirwalk.Dirent) error {
			if skip(de.Name()) {
				return godirwalk.SkipThis
			}
			out = append(out, relative(root, path))
			return nil
		},
	})
	return out, err
}

func collectWalkDirsSkipThis(root string, skip func(string) bool) ([]string, error) {
	var out []string
	err := treestamp.WalkDirs(root, treestamp.DirWalkOptions{
		Callback: func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if skip(d.Name()) {
				return treestamp.SkipThis
			}
			out = append(out, relative(root, path))
			return nil
		},
	})
	return out, err
}

func collectCompatSkipThis(root string, skip func(string) bool) ([]string, error) {
	var out []string
	err := tsgodirwalk.Walk(root, &tsgodirwalk.Options{
		Callback: func(path string, de *tsgodirwalk.Dirent) error {
			if skip(de.Name()) {
				return tsgodirwalk.SkipThis
			}
			out = append(out, relative(root, path))
			return nil
		},
	})
	return out, err
}

func assertPostChildrenParity(t *testing.T, root string) {
	t.Helper()
	want, err := collectGodirwalkPost(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := collectWalkDirsPost(root)
	if err != nil || !equalStrings(want, got) {
		t.Fatalf("WalkDirs PostChildren\nwant %#v\n got %#v err=%v", want, got, err)
	}
	alias, err := collectCompatPost(root)
	if err != nil || !equalStrings(want, alias) {
		t.Fatalf("compat PostChildren\nwant %#v\n got %#v err=%v", want, alias, err)
	}
}

func collectGodirwalkPost(root string) ([]string, error) {
	var out []string
	err := godirwalk.Walk(root, &godirwalk.Options{
		Callback: func(string, *godirwalk.Dirent) error { return nil },
		PostChildrenCallback: func(path string, _ *godirwalk.Dirent) error {
			out = append(out, relative(root, path))
			return nil
		},
	})
	return out, err
}

func collectWalkDirsPost(root string) ([]string, error) {
	var out []string
	err := treestamp.WalkDirs(root, treestamp.DirWalkOptions{
		Callback: func(string, fs.DirEntry, error) error { return nil },
		PostChildrenCallback: func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			out = append(out, relative(root, path))
			return nil
		},
	})
	return out, err
}

func collectCompatPost(root string) ([]string, error) {
	var out []string
	err := tsgodirwalk.Walk(root, &tsgodirwalk.Options{
		Callback: func(string, *tsgodirwalk.Dirent) error { return nil },
		PostChildrenCallback: func(path string, _ *tsgodirwalk.Dirent) error {
			out = append(out, relative(root, path))
			return nil
		},
	})
	return out, err
}

func assertScannerParity(t *testing.T, root string, scratch []byte) {
	t.Helper()
	want := collectGodirwalkScanner(t, root, scratch)
	got := collectTreestampScanner(t, root, scratch)
	if !equalStrings(want, got) {
		t.Fatalf("DirScanner\nwant %#v\n got %#v", want, got)
	}
	alias := collectCompatScanner(t, root, scratch)
	if !equalStrings(want, alias) {
		t.Fatalf("compat Scanner\nwant %#v\n got %#v", want, alias)
	}
}

func collectCompatScanner(t *testing.T, root string, scratch []byte) []string {
	t.Helper()
	sc, err := tsgodirwalk.NewScannerWithScratchBuffer(root, scratch)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for sc.Scan() {
		de, err := sc.Dirent()
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, de.Name()+":"+de.ModeType().String())
	}
	if err := sc.Err(); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func assertAllowNonDirectoryParity(t *testing.T, file string) {
	t.Helper()
	want := godirwalk.Walk(file, &godirwalk.Options{
		Callback: func(string, *godirwalk.Dirent) error { return nil },
	})
	got := treestamp.WalkDirs(file, treestamp.DirWalkOptions{
		Callback: func(string, fs.DirEntry, error) error { return nil },
	})
	if want == nil || got == nil || !strings.Contains(got.Error(), "cannot Walk non-directory") {
		t.Fatalf("file root want=%v got=%v", want, got)
	}
	if err := treestamp.WalkDirs(file, treestamp.DirWalkOptions{
		AllowNonDirectory: true,
		Callback:          func(string, fs.DirEntry, error) error { return nil },
	}); err != nil {
		t.Fatal(err)
	}
}

func assertErrorCallbackParity(t *testing.T, root string) {
	t.Helper()
	boom := errors.New("boom")
	halt := godirwalk.Walk(root, &godirwalk.Options{
		Callback: func(_ string, de *godirwalk.Dirent) error {
			if de.Name() == "a.txt" {
				return boom
			}
			return nil
		},
		ErrorCallback: func(string, error) godirwalk.ErrorAction { return godirwalk.Halt },
	})
	if !errors.Is(halt, boom) {
		t.Fatalf("godirwalk Halt %v", halt)
	}
	skip := tsgodirwalk.Walk(root, &tsgodirwalk.Options{
		Callback: func(_ string, de *tsgodirwalk.Dirent) error {
			if de.Name() == "a.txt" {
				return boom
			}
			return nil
		},
		ErrorCallback: func(string, error) tsgodirwalk.ErrorAction { return tsgodirwalk.SkipNode },
	})
	if skip != nil {
		t.Fatalf("compat SkipNode %v", skip)
	}
}

func assertScratchFloor(t *testing.T) {
	t.Helper()
	if godirwalk.MinimumScratchBufferSize != tsgodirwalk.MinimumScratchBufferSize {
		t.Fatalf("scratch %d %d", godirwalk.MinimumScratchBufferSize, tsgodirwalk.MinimumScratchBufferSize)
	}
}

func assertFollowParity(t *testing.T, root string) {
	t.Helper()
	mustSymlink(t, "keep", filepath.Join(root, "alias"))
	want, err := collectGodirwalkFollow(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := walkDirsFollow(root)
	if err != nil || !equalStrings(want, got) {
		t.Fatalf("Follow\nwant %#v\n got %#v err=%v", want, got, err)
	}
}

func collectGodirwalkFollow(root string) ([]string, error) {
	var out []string
	err := godirwalk.Walk(root, &godirwalk.Options{
		FollowSymbolicLinks: true,
		Callback: func(path string, _ *godirwalk.Dirent) error {
			out = append(out, relative(root, path))
			return nil
		},
	})
	return out, err
}

func walkDirsFollow(root string) ([]string, error) {
	var out []string
	err := treestamp.WalkDirs(root, treestamp.DirWalkOptions{
		FollowSymbolicLinks: true,
		Callback: func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			out = append(out, relative(root, path))
			return nil
		},
	})
	return out, err
}

func BenchmarkWalkDirsSkipThisTreestamp(b *testing.B) {
	root := makeWideDir(b, 400)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := collectWalkDirsSkipThis(root, skipSub); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWalkDirsSkipThisGodirwalk(b *testing.B) {
	root := makeWideDir(b, 400)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := collectGodirwalkSkipThis(root, skipSub); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWalkDirsPostChildrenTreestamp(b *testing.B) {
	root := makeWideDir(b, 400)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := collectWalkDirsPost(root); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWalkDirsPostChildrenGodirwalk(b *testing.B) {
	root := makeWideDir(b, 400)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := collectGodirwalkPost(root); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNewDirentGodirwalk(b *testing.B) {
	root := makeWideDir(b, 8)
	file := filepath.Join(root, "f0000.txt")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := godirwalk.NewDirent(file); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNewDirentCompat(b *testing.B) {
	root := makeWideDir(b, 8)
	file := filepath.Join(root, "f0000.txt")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := tsgodirwalk.NewDirent(file); err != nil {
			b.Fatal(err)
		}
	}
}

func skipSub(name string) bool { return name == "sub" }
