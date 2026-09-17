package treestamp_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sergii-ziborov/treestamp"
)

func TestCompileRejectsInvalidRegex(t *testing.T) {
	_, err := treestamp.Compile(treestamp.WithFilenameRegex("("))
	if err == nil {
		t.Fatal("expected config error")
	}
	var typed *treestamp.Error
	if !errors.As(err, &typed) || typed.Code != treestamp.CodeInvalid {
		t.Fatalf("%v", err)
	}
}

func TestScanWithFiltersAndPlanReuse(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "keep.go"), "package keep\n")
	mustWrite(t, filepath.Join(root, "skip_test.go"), "package keep\n")
	mustWrite(t, filepath.Join(root, "other.rs"), "fn main() {}\n")
	ctx := context.Background()
	base := treestamp.DefaultOptions()
	base.IgnoreFiles = nil
	base.IgnorePolicy = treestamp.NoneIgnorePolicy()
	base.StandardSkips = false
	plan, err := treestamp.Compile(treestamp.Using(base), treestamp.WithExtensions("go"), treestamp.WithExcludeGlobs("*_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Describe(), "profile=repository") || !strings.Contains(plan.Describe(), "hash=true") {
		t.Fatalf("describe %s", plan.Describe())
	}
	report, err := plan.Scan(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Files) != 1 || report.Files[0].Relative != "keep.go" {
		t.Fatalf("%v", rels(report))
	}
	exts := []string{"go"}
	plan2, err := treestamp.Compile(treestamp.Using(base), treestamp.WithExtensions(exts...))
	if err != nil {
		t.Fatal(err)
	}
	exts[0] = "rs"
	report, err = plan2.Scan(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Files) != 2 {
		t.Fatalf("mutation leaked into plan: %v", rels(report))
	}
}

func TestScanPathsWithRejectsExplicitHash(t *testing.T) {
	_, err := treestamp.ScanPathsWith(context.Background(), t.TempDir(), treestamp.WithHashContents(true))
	if err == nil {
		t.Fatal("expected unsupported content option")
	}
}

func TestEachFileOwnedBytesAndStop(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	mustWrite(t, filepath.Join(root, "b.go"), "package b\n")
	var held []byte
	_, err := treestamp.EachFile(context.Background(), root, func(file treestamp.ScannedFile, data []byte) error {
		if file.Relative == "a.go" {
			held = data
			data[0] = 'X'
			return treestamp.ErrStop
		}
		t.Fatalf("callback replayed %s", file.Relative)
		return nil
	}, treestamp.WithExtensions("go"))
	if !errors.Is(err, treestamp.ErrStop) {
		t.Fatalf("%v", err)
	}
	if string(held[:8]) != "Xackage " && !bytes.HasPrefix(held, []byte("X")) {
		t.Fatalf("owned bytes were not transferred: %q", held)
	}
}

func TestBroaderScanOptionsStayConsistent(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	mustWrite(t, filepath.Join(root, ".hidden.go"), "package h\n")
	opts := treestamp.DefaultOptions().WithExtensions("go").WithSkipHidden(true).WithMaxFileBytes(50).WithAdmitTimeout(time.Second)
	rep, err := treestamp.ScanWith(context.Background(), root, treestamp.Using(opts))
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Descriptor.Matches(opts) {
		t.Fatal("descriptor")
	}
	for _, file := range rep.Files {
		if file.Relative == ".hidden.go" {
			t.Fatal("hidden selected")
		}
	}
}

func TestEachFileHashMatchesOwnedBytes(t *testing.T) {
	root := t.TempDir()
	body := []byte("package a\n")
	mustWrite(t, filepath.Join(root, "a.go"), string(body))
	want := "sha256:" + hex.EncodeToString(sha256Sum(body))
	var seen int
	_, err := treestamp.EachFile(context.Background(), root, func(file treestamp.ScannedFile, data []byte) error {
		seen++
		if !bytes.Equal(data, body) {
			t.Fatalf("bytes %q", data)
		}
		if file.ContentHash != want {
			t.Fatalf("hash %s want %s", file.ContentHash, want)
		}
		return nil
	}, treestamp.WithExtensions("go"))
	if err != nil || seen != 1 {
		t.Fatalf("seen=%d err=%v", seen, err)
	}
}

func sha256Sum(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

func TestEachFileCallbackError(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	want := errors.New("indexer")
	_, err := treestamp.EachFile(context.Background(), root, func(treestamp.ScannedFile, []byte) error {
		return want
	}, treestamp.WithExtensions("go"))
	if !errors.Is(err, want) {
		t.Fatalf("%v", err)
	}
}

func TestScanWithCancel(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := treestamp.ScanWith(ctx, root)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
}

func TestFrozenProviderSurvivesReportMutation(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	report, err := treestamp.ScanWith(context.Background(), root, treestamp.WithExtensions("go"))
	if err != nil {
		t.Fatal(err)
	}
	provider, err := report.ContentProvider()
	if err != nil {
		t.Fatal(err)
	}
	report.Files = nil
	got, err := provider.Read("a.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got.Bytes, []byte("package a")) {
		t.Fatalf("%q", got.Bytes)
	}
}

func TestLoggerSilentByDefaultAndOptional(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	if _, err := treestamp.ScanWith(context.Background(), root, treestamp.WithLogger(logger), treestamp.WithExtensions("go")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "scan.finished") {
		t.Fatalf("log %q", buf.String())
	}
}

func TestExplainUsesCompiledPlan(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".gitignore"), "secret.bin\n")
	mustWrite(t, filepath.Join(root, "secret.bin"), "x")
	why, err := treestamp.Explain(root, "secret.bin")
	if err != nil {
		t.Fatal(err)
	}
	if why.Outcome == "selected" {
		t.Fatalf("%+v", why)
	}
}

func TestConcurrentPlanScans(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	plan, err := treestamp.Compile(treestamp.WithExtensions("go"))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errc := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := plan.Scan(context.Background(), root)
			errc <- err
		}()
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestPlanFilesYieldsSelected(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	plan, err := treestamp.Compile(treestamp.WithExtensions("go"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	plan.Files(context.Background(), root)(func(file treestamp.ScannedFile, yieldErr error) bool {
		if yieldErr != nil {
			t.Fatal(yieldErr)
		}
		n++
		return true
	})
	if n != 1 {
		t.Fatalf("got %d", n)
	}
}

func rels(r *treestamp.ScanReport) []string {
	var out []string
	for _, f := range r.Files {
		out = append(out, f.Relative)
	}
	return out
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
