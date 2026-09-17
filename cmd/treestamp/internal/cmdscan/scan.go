package cmdscan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sergii-ziborov/treestamp"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/app"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/policy"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/render"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/status"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/store"
	"github.com/spf13/cobra"
)

func New(env *app.Env) *cobra.Command {
	var sel policy.Select
	cmd := &cobra.Command{
		Use:   "scan [ROOT]",
		Short: "Scan the selected tree and print a summary or manifest",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			return run(cmd.Context(), env, sel, root)
		},
	}
	sel.Bind(cmd)
	return cmd
}

func run(ctx context.Context, env *app.Env, sel policy.Select, root string) error {
	if err := sel.ApplyConfig(); err != nil {
		return env.Fail(status.Usage, "config: %v", err)
	}
	opts, err := sel.Options()
	if err != nil {
		return env.Fail(status.Usage, "%v", err)
	}
	if sel.Output != "" {
		inside, err := store.InsideRoot(root, sel.Output)
		if err != nil {
			return env.Fail(status.Impossible, "%v", err)
		}
		if inside {
			return env.Fail(status.Usage, "refusing --output inside scan root; write the manifest outside the tree")
		}
	}
	rep, err := treestamp.ScanWith(ctx, root, treestamp.Using(opts))
	if errors.Is(err, treestamp.ErrPartial) {
		env.Set(status.Partial)
	} else if err != nil {
		return env.Fail(mapScan(err), "%v", err)
	}
	if rep == nil {
		return env.Fail(status.Impossible, "scan produced no report")
	}
	man := store.FromReport(rep, sel.Snapshot(opts))
	return publish(env, sel, man, err)
}

func publish(env *app.Env, sel policy.Select, man store.Manifest, scanErr error) error {
	payload, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return env.Fail(status.Publish, "encode: %v", err)
	}
	payload = append(bytes.TrimRight(payload, "\n"), '\n')
	if sel.Output != "" {
		if !man.Observation.Complete || env.Code == status.Partial {
			fmt.Fprintln(env.Err, "output baseline was not replaced")
		} else if err := store.WriteAtomic(sel.Output, payload); err != nil {
			return env.Fail(status.Publish, "write output: %v", err)
		}
	}
	switch sel.FormatName() {
	case "json":
		if _, err := env.Out.Write(payload); err != nil {
			return env.Fail(status.Publish, "stdout: %v", err)
		}
	case "ndjson":
		return writeNDJSON(env, man)
	case "text":
		if err := writeHuman(env, sel, man); err != nil {
			return env.Fail(status.Publish, "stdout: %v", err)
		}
	default:
		return env.Fail(status.Usage, "unknown format %q", sel.FormatName())
	}
	if scanErr != nil && env.Code == 0 {
		env.Set(status.Partial)
	}
	return nil
}

func writeHuman(env *app.Env, sel policy.Select, man store.Manifest) error {
	pal := render.Detect(env.Out, firstNonEmpty(env.Color, sel.Color))
	tone, title := "ok", "COMPLETE"
	if !man.Observation.Complete || man.Summary.Failures > 0 {
		tone, title = "warn", "PARTIAL"
		if env.Code == 0 {
			env.Set(status.Partial)
		}
	}
	return render.WriteCard(env.Out, pal, render.Card{
		Status: title, Detail: "within selected scope", Tone: tone,
		Rows: []render.Row{
			{Key: "Selected", Value: render.Comma(man.Summary.Selected) + " files"},
			{Key: "Hashed", Value: render.Comma(man.Summary.Hashed) + " files"},
			{Key: "Excluded", Value: render.Comma(man.Summary.Excluded)},
			{Key: "Failures", Value: render.Comma(man.Summary.Failures)},
			{Key: "Profile", Value: man.Policy.Profile},
			{Key: "Revision", Value: man.Revisions.Legacy},
		},
		Notes: []string{"Policy exclusions are not operational failures."},
	})
}

func writeNDJSON(env *app.Env, man store.Manifest) error {
	begin := map[string]any{"schema": store.Schema, "event": "scan_begin", "profile": man.Policy.Profile}
	if err := render.JSONLine(env.Out, begin); err != nil {
		return env.Fail(status.Publish, "stdout: %v", err)
	}
	for _, file := range man.Files {
		if err := render.JSONLine(env.Out, map[string]any{"event": "file_committed", "relative": file.Relative, "sha256": file.Hash}); err != nil {
			return env.Fail(status.Publish, "stdout: %v", err)
		}
	}
	end := map[string]any{"event": "scan_end", "complete": man.Observation.Complete, "revisions": man.Revisions, "summary": man.Summary}
	if err := render.JSONLine(env.Out, end); err != nil {
		return env.Fail(status.Publish, "stdout: %v", err)
	}
	return nil
}

func mapScan(err error) int {
	var te *treestamp.Error
	if errors.As(err, &te) && te.Code == treestamp.CodeTimeout {
		return status.Timeout
	}
	return status.Impossible
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
