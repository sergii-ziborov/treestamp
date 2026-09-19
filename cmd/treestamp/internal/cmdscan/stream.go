package cmdscan

import (
	"context"
	"errors"
	"fmt"

	"github.com/sergii-ziborov/treestamp"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/app"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/policy"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/render"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/status"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/store"
)

func runNDJSON(ctx context.Context, env *app.Env, sel policy.Select, root string, opts treestamp.Options) error {
	snap := sel.Snapshot(opts)
	if err := render.JSONLine(env.Out, map[string]any{"schema": store.Schema, "event": "scan_begin", "profile": snap.Profile}); err != nil {
		return env.Fail(status.Publish, "stdout: %v", err)
	}
	acc := &ndjsonAcc{keep: sel.Output != ""}
	stream, err := treestamp.ScanIntoErr(ctx, root, acc.commit(env), treestamp.Using(opts))
	if acc.writeErr != nil {
		return env.Fail(status.Publish, "stdout: %v", acc.writeErr)
	}
	if errors.Is(err, treestamp.ErrPartial) {
		env.Set(status.Partial)
	} else if err != nil {
		return env.Fail(mapScan(err), "%v", err)
	}
	man := acc.manifest(stream, snap)
	if err := writeScanEnd(env, man); err != nil {
		return err
	}
	return writeBaseline(env, sel, man)
}

type ndjsonAcc struct {
	keep     bool
	hashed   int
	files    []treestamp.ScannedFile
	writeErr error
}

func (a *ndjsonAcc) commit(env *app.Env) func(*treestamp.ScannedFile) error {
	return func(file *treestamp.ScannedFile) error {
		if a.writeErr != nil {
			return a.writeErr
		}
		a.writeErr = render.JSONLine(env.Out, map[string]any{
			"event": "file_committed", "relative": file.Relative, "sha256": file.ContentHash,
		})
		if file.ContentHash != "" {
			a.hashed++
		}
		if a.keep {
			a.files = append(a.files, *file)
		}
		return a.writeErr
	}
}

func (a *ndjsonAcc) manifest(stream *treestamp.ScanStreamReport, snap policy.Snapshot) store.Manifest {
	if a.keep && stream != nil {
		return store.FromReport(reportFromStream(stream, a.files), snap)
	}
	man := store.FromReport(&treestamp.ScanReport{}, snap)
	if stream == nil {
		man.Observation.Complete = false
		return man
	}
	man.Observation.Complete = stream.Complete
	man.Observation.Termination = stream.Termination.String()
	man.Observation.Portable = stream.Portable
	man.Revisions.Legacy = stream.Revision
	man.Summary.Selected = int(stream.Emitted)
	man.Summary.Hashed = a.hashed
	man.Summary.Excluded = len(stream.Skipped)
	for _, skipped := range stream.Skipped {
		if skipped.Kind == treestamp.SkipIOError || skipped.Kind == treestamp.SkipConcurrentModification {
			man.Summary.Failures++
		}
	}
	return man
}

func reportFromStream(stream *treestamp.ScanStreamReport, files []treestamp.ScannedFile) *treestamp.ScanReport {
	return &treestamp.ScanReport{
		Root: stream.Root, Files: files, Skipped: stream.Skipped, Warnings: stream.Warnings,
		IgnoreSources: stream.IgnoreSources, Revision: stream.Revision,
		Complete: stream.Complete, Termination: stream.Termination,
		Portable: stream.Portable, Cache: stream.Cache,
	}
}

func writeScanEnd(env *app.Env, man store.Manifest) error {
	if err := render.JSONLine(env.Out, map[string]any{
		"event": "scan_end", "complete": man.Observation.Complete, "revisions": man.Revisions, "summary": man.Summary,
	}); err != nil {
		return env.Fail(status.Publish, "stdout: %v", err)
	}
	return nil
}

func writeBaseline(env *app.Env, sel policy.Select, man store.Manifest) error {
	if sel.Output == "" {
		return nil
	}
	if !man.Observation.Complete || env.Code == status.Partial {
		fmt.Fprintln(env.Err, "output baseline was not replaced")
		return nil
	}
	payload, err := encodeManifest(man)
	if err != nil {
		return env.Fail(status.Publish, "encode: %v", err)
	}
	if err := store.WriteAtomic(sel.Output, payload); err != nil {
		return env.Fail(status.Publish, "write output: %v", err)
	}
	return nil
}
