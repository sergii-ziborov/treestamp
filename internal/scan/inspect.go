package scan

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"io"
	"os"
	"runtime"
	"sort"
	"sync"

	"github.com/sergii-ziborov/treestamp/internal/filetypes"
	"github.com/sergii-ziborov/treestamp/internal/hashx"
	"github.com/sergii-ziborov/treestamp/internal/ignore"
	"github.com/sergii-ziborov/treestamp/internal/selection"
	"github.com/sergii-ziborov/treestamp/internal/walk"
)

type inspectResult struct {
	file ScannedFile
	skip *Skipped
	stat CacheStats
	err  error
}

func inspect(ctx context.Context, files []candidate, opts Options) ([]ScannedFile, []Skipped, CacheStats, error) {
	index := cacheIndex(opts)
	workers := opts.ContentWorkers
	if workers <= 0 {
		workers = 1
		if opts.HashFileContents && len(files) > 8 {
			workers = min(runtime.GOMAXPROCS(0), 4)
		}
	}
	runOne := func(c candidate) inspectResult {
		file, skip, stat, err := inspectOne(c, opts, index)
		return inspectResult{file: file, skip: skip, stat: stat, err: err}
	}
	if workers == 1 {
		return inspectSerial(ctx, files, runOne)
	}
	return inspectParallel(ctx, files, workers, runOne)
}

func inspectSerial(ctx context.Context, files []candidate, runOne func(candidate) inspectResult) ([]ScannedFile, []Skipped, CacheStats, error) {
	var out []ScannedFile
	var skipped []Skipped
	var stats CacheStats
	for _, c := range files {
		if err := ctx.Err(); err != nil {
			return nil, nil, stats, err
		}
		item := runOne(c)
		if item.err != nil {
			return nil, nil, stats, item.err
		}
		applyInspectResult(inspectOut{files: &out, skipped: &skipped, stats: &stats}, item, false)
	}
	return out, skipped, stats, nil
}

func inspectParallel(ctx context.Context, files []candidate, workers int, runOne func(candidate) inspectResult) ([]ScannedFile, []Skipped, CacheStats, error) {
	ch := make(chan candidate)
	res := make(chan inspectResult, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range ch {
				res <- runOne(c)
			}
		}()
	}
	go func() {
		defer close(ch)
		for _, c := range files {
			select {
			case <-ctx.Done():
				return
			case ch <- c:
			}
		}
	}()
	go func() { wg.Wait(); close(res) }()
	var out []ScannedFile
	var skipped []Skipped
	var stats CacheStats
	for item := range res {
		if item.err != nil {
			return nil, nil, stats, item.err
		}
		applyInspectResult(inspectOut{files: &out, skipped: &skipped, stats: &stats}, item, false)
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, stats, err
	}
	return out, skipped, stats, nil
}

func inspectCompact(ctx context.Context, files []candidate, opts Options) ([]CompactFile, []Skipped, CacheStats, error) {
	index := cacheIndex(opts)
	var compact []CompactFile
	var skipped []Skipped
	var stats CacheStats
	for _, c := range files {
		if err := ctx.Err(); err != nil {
			return nil, nil, stats, err
		}
		file, skip, stat, err := inspectOne(c, opts, index)
		if err != nil {
			return nil, nil, stats, err
		}
		applyInspectResult(inspectOut{compact: &compact, skipped: &skipped, stats: &stats}, inspectResult{file: file, skip: skip, stat: stat}, true)
	}
	return compact, skipped, stats, nil
}

type inspectOut struct {
	files   *[]ScannedFile
	compact *[]CompactFile
	skipped *[]Skipped
	stats   *CacheStats
}

func applyInspectResult(out inspectOut, item inspectResult, asCompact bool) {
	out.stats.ReusedHashes += item.stat.ReusedHashes
	out.stats.ContentReads += item.stat.ContentReads
	out.stats.FingerprintReads += item.stat.FingerprintReads
	if item.skip != nil {
		*out.skipped = append(*out.skipped, *item.skip)
		return
	}
	if asCompact {
		*out.compact = append(*out.compact, compactFrom(item.file))
		return
	}
	*out.files = append(*out.files, item.file)
}

func cacheIndex(opts Options) map[string]CacheEntry {
	index := map[string]CacheEntry{}
	if opts.Cache != nil {
		for _, entry := range opts.Cache.Entries {
			index[entry.Relative] = entry
		}
	}
	return index
}

func inspectOne(c candidate, opts Options, index map[string]CacheEntry) (ScannedFile, *Skipped, CacheStats, error) {
	file := ScannedFile{Absolute: c.abs, Relative: c.rel, Bytes: c.size, Version: c.version}
	if reused, skip, stat, ok := reuseCached(c, opts, index, &file); ok {
		return file, skip, stat, reused
	}
	if !opts.HashFileContents && !opts.DetectBinary {
		return file, nil, CacheStats{}, nil
	}
	return hashOpened(c, opts, file)
}

func reuseCached(c candidate, opts Options, index map[string]CacheEntry, file *ScannedFile) (error, *Skipped, CacheStats, bool) {
	cached, ok := index[c.rel]
	if !ok || cached.Bytes != c.size || !cached.Version.Reusable(c.version) {
		return nil, nil, CacheStats{}, false
	}
	if opts.CacheValidation == CacheFast {
		file.ContentHash, file.ContentFingerprint, file.BinaryChecked = cached.ContentHash, cached.ContentFingerprint, cached.BinaryChecked
		return nil, nil, CacheStats{ReusedHashes: 1}, true
	}
	fp, err := fingerprintFile(c.abs)
	if err == nil && fp == cached.ContentFingerprint {
		file.ContentHash, file.ContentFingerprint, file.BinaryChecked = cached.ContentHash, cached.ContentFingerprint, cached.BinaryChecked
		return nil, nil, CacheStats{ReusedHashes: 1, FingerprintReads: 1}, true
	}
	return nil, nil, CacheStats{}, false
}

func hashOpened(c candidate, opts Options, file ScannedFile) (ScannedFile, *Skipped, CacheStats, error) {
	f, err := os.Open(c.abs)
	if err != nil {
		return file, &Skipped{Relative: c.rel, Kind: selection.SkipIOError, Detail: err.Error()}, CacheStats{}, nil
	}
	defer f.Close()
	var h hash.Hash
	if opts.HashFileContents {
		h = sha256.New()
	}
	fp := hashx.NewContentFingerprint()
	needFP := opts.HashFileContents || opts.CacheValidation == CacheStrict
	buf := make([]byte, 64*1024)
	var read uint64
	for {
		n, readErr := f.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if opts.DetectBinary && bytes.IndexByte(chunk, 0) >= 0 {
				return file, &Skipped{Relative: c.rel, Kind: selection.SkipBinary}, CacheStats{ContentReads: 1}, nil
			}
			if h != nil {
				_, _ = h.Write(chunk)
			}
			if needFP {
				fp.Write(chunk)
			}
			read += uint64(n)
			if !opts.HashFileContents && read >= 8*1024 {
				break
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return file, &Skipped{Relative: c.rel, Kind: selection.SkipIOError, Detail: readErr.Error()}, CacheStats{ContentReads: 1}, nil
		}
	}
	if skip := checkContentSize(f, c, opts); skip != nil {
		return file, skip, CacheStats{ContentReads: 1}, nil
	}
	if h != nil {
		file.ContentHash = hashx.Finish(h)
	}
	if needFP {
		file.ContentFingerprint = fp.Finish()
	}
	file.BinaryChecked = opts.DetectBinary
	return file, nil, CacheStats{ContentReads: 1}, nil
}

func checkContentSize(f *os.File, c candidate, opts Options) *Skipped {
	if opts.ContentValidation != ContentStrict {
		return nil
	}
	info, statErr := f.Stat()
	if statErr == nil && uint64(info.Size()) != c.size {
		return &Skipped{Relative: c.rel, Kind: selection.SkipConcurrentModification}
	}
	return nil
}

func fingerprintFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	fp := hashx.NewContentFingerprint()
	buf := make([]byte, 64*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			fp.Write(buf[:n])
		}
		if err == io.EOF {
			return fp.Finish(), nil
		}
		if err != nil {
			return "", err
		}
	}
}

func compactFrom(file ScannedFile) CompactFile {
	item := CompactFile{Relative: file.Relative, Bytes: file.Bytes}
	if file.ContentHash != "" || file.ContentFingerprint != "" || file.BinaryChecked {
		item.Content = &ContentEvidence{
			ContentHash: file.ContentHash, ContentFingerprint: file.ContentFingerprint,
			Version: file.Version, BinaryChecked: file.BinaryChecked,
		}
	}
	return item
}

func DescriptorFromOptions(opts Options) Descriptor {
	h := sha256.New()
	writeDescriptorCore(h, opts)
	return Descriptor{Version: DescriptorVersion, Policy: "sha256:" + hex.EncodeToString(h.Sum(nil))}
}

func writeDescriptorCore(h hash.Hash, opts Options) {
	_, _ = h.Write([]byte("scan-policy"))
	var ver [4]byte
	binary.LittleEndian.PutUint32(ver[:], DescriptorVersion)
	_, _ = h.Write(ver[:])
	exts := append([]string(nil), opts.Extensions...)
	sort.Strings(exts)
	writeStrings(h, exts)
	if opts.FileTypes != nil {
		opts.FileTypes.WritePolicy(h)
	} else {
		filetypes.New().WritePolicy(h)
	}
	writeStrings(h, opts.OverrideRules)
	writeStrings(h, opts.IgnoreFiles)
	writeIgnorePolicy(h, opts)
	writeDescriptorFlags(h, opts)
}

func writeDescriptorFlags(h hash.Hash, opts Options) {
	writeBool(h, opts.IgnoreCase)
	writeBool(h, opts.SkipHidden)
	writeByteFlag(h, opts.StandardSkips, 1, 0)
	writeBool(h, opts.HashFileContents)
	writeBool(h, opts.DetectBinary)
	writeByteFlag(h, opts.RecordSkipped, 1, 2)
	var max [8]byte
	binary.LittleEndian.PutUint64(max[:], opts.MaxFileBytes)
	_, _ = h.Write(max[:])
	writeOptionalU64(h, opts.Limits.MaxEntries)
	writeOptionalU64(h, opts.Limits.MaxTotalBytes)
	writeWalkPolicy(h, opts.Walk)
}

func writeWalkPolicy(h hash.Hash, w walk.WalkOptions) {
	var min [8]byte
	binary.LittleEndian.PutUint64(min[:], uint64(w.MinDepth))
	_, _ = h.Write(min[:])
	if w.MaxDepth == nil {
		_, _ = h.Write([]byte{0})
	} else {
		_, _ = h.Write([]byte{1})
		var d [8]byte
		binary.LittleEndian.PutUint64(d[:], uint64(*w.MaxDepth))
		_, _ = h.Write(d[:])
	}
	writeBool(h, w.FollowLinks)
	writeBool(h, w.SameFileSystem)
	writeByteFlag(h, w.RootSymlinkPolicy == walk.RootReject, 2, 1)
	writeByteFlag(h, w.ErrorPolicy == walk.ErrorAbort, 2, 1)
}

func writeByteFlag(h hash.Hash, cond bool, yes, no byte) {
	if cond {
		_, _ = h.Write([]byte{yes})
	} else {
		_, _ = h.Write([]byte{no})
	}
}

func (d Descriptor) Matches(opts Options) bool {
	other := DescriptorFromOptions(opts)
	return d.Version == DescriptorVersion && d.Policy == other.Policy
}

func writeStrings(h hash.Hash, values []string) {
	for _, value := range values {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{0})
	}
	_, _ = h.Write([]byte{0xfe})
}

func writeBool(h hash.Hash, v bool) { writeByteFlag(h, v, 1, 0) }

func writeOptionalU64(h hash.Hash, value *uint64) {
	if value == nil {
		_, _ = h.Write([]byte{0})
		return
	}
	_, _ = h.Write([]byte{1})
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], *value)
	_, _ = h.Write(buf[:])
}

func writeIgnorePolicy(h hash.Hash, opts Options) {
	p := opts.IgnorePolicy
	if !p.Specified() {
		p = ignore.RepositoryPolicy()
	}
	writeBool(h, p.ParentRules)
	writeBool(h, p.GitIgnore)
	writeBool(h, p.DotIgnore)
	writeBool(h, p.CustomIgnore)
	writeBool(h, p.GitExclude)
	writeBool(h, p.GitGlobal)
	writeBool(h, p.RequireGit)
	for _, path := range p.ExplicitFiles {
		_, _ = h.Write([]byte(path))
		_, _ = h.Write([]byte{0})
	}
	_, _ = h.Write([]byte{0xfe})
}

func finalize(report *Report, opts Options) {
	sort.Slice(report.Files, func(i, j int) bool { return report.Files[i].Relative < report.Files[j].Relative })
	sort.Slice(report.Skipped, func(i, j int) bool {
		if report.Skipped[i].Relative != report.Skipped[j].Relative {
			return report.Skipped[i].Relative < report.Skipped[j].Relative
		}
		return report.Skipped[i].Kind < report.Skipped[j].Kind
	})
	sort.Slice(report.Warnings, func(i, j int) bool {
		if report.Warnings[i].Relative != report.Warnings[j].Relative {
			return report.Warnings[i].Relative < report.Warnings[j].Relative
		}
		return report.Warnings[i].Message < report.Warnings[j].Message
	})
	sort.Slice(report.IgnoreSources, func(i, j int) bool {
		if report.IgnoreSources[i].Kind != report.IgnoreSources[j].Kind {
			return report.IgnoreSources[i].Kind < report.IgnoreSources[j].Kind
		}
		return report.IgnoreSources[i].Location < report.IgnoreSources[j].Location
	})
	report.Descriptor = DescriptorFromOptions(opts)
	report.Revision = revision(report.Files, report.IgnoreSources, report.Portable, report.Termination)
}

type revisionBuilder struct{ h hash.Hash }

func newRevisionBuilder(sources []IgnoreSource) *revisionBuilder {
	h := sha256.New()
	_, _ = h.Write([]byte("scan-revision\x01"))
	for _, source := range sources {
		_, _ = h.Write([]byte{byte(source.Kind), 0})
		_, _ = h.Write([]byte(source.Location))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(source.ContentHash))
		_, _ = h.Write([]byte{0xfe})
	}
	return &revisionBuilder{h: h}
}

func (b *revisionBuilder) push(file ScannedFile) {
	if b == nil {
		return
	}
	_, _ = b.h.Write([]byte(file.Relative))
	_, _ = b.h.Write([]byte{0})
	var size [8]byte
	binary.LittleEndian.PutUint64(size[:], file.Bytes)
	_, _ = b.h.Write(size[:])
	_, _ = b.h.Write([]byte{0})
	_, _ = b.h.Write([]byte(file.ContentHash))
	_, _ = b.h.Write([]byte{0xff})
}

func (b *revisionBuilder) finish(portable bool, term Termination) string {
	writeByteFlag(b.h, portable, 1, 0)
	if term != TermNone {
		_, _ = b.h.Write([]byte{byte(term)})
	}
	return "sha256:" + hex.EncodeToString(b.h.Sum(nil))
}

func revision(files []ScannedFile, sources []IgnoreSource, portable bool, term Termination) string {
	builder := newRevisionBuilder(sources)
	for _, file := range files {
		builder.push(file)
	}
	return builder.finish(portable, term)
}

func compactRevision(files []CompactFile, sources []IgnoreSource, portable bool, term Termination) string {
	converted := make([]ScannedFile, len(files))
	for i, file := range files {
		converted[i] = ScannedFile{Relative: file.Relative, Bytes: file.Bytes, ContentHash: file.ContentHash()}
	}
	return revision(converted, sources, portable, term)
}
