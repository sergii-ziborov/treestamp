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
		Short: "Hash selected files and print a summary",
		Long:  "Scan ROOT (default .) and print what was selected. --output must be outside ROOT.",
		Example: "  treestamp scan ./cmd/treestamp --ext go --output ./baselines/cli.tstamp.json\n" +
			"  treestamp scan . --ext go --output ../repo.tstamp.json",
		Args: cobra.MaximumNArgs(1),
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
	tone, title := "ok", "Complete"
	if !man.Observation.Complete || man.Summary.Failures > 0 {
		tone, title = "warn", "Partial"
		if env.Code == 0 {
			env.Set(status.Partial)
		}
	}
	card := render.Card{Status: title, Detail: scanDetail(man), Tone: tone, Rows: scanRows(sel, man)}
	if !sel.Quiet && sel.Output == "" {
		card.Next = []string{"Save a baseline:  treestamp scan <root> --output FILE"}
	}
	return render.WriteCard(env.Out, pal, card)
}

func scanDetail(man store.Manifest) string {
	n := render.Comma(man.Summary.Selected)
	if man.Summary.Selected == 0 {
		return "no files selected"
	}
	if man.Summary.Hashed == man.Summary.Selected {
		return n + " files selected and hashed"
	}
	if man.Summary.Hashed == 0 {
		return n + " files selected (not hashed)"
	}
	return n + " selected, " + render.Comma(man.Summary.Hashed) + " hashed"
}

func scanRows(sel policy.Select, man store.Manifest) []render.Row {
	rows := []render.Row{{Key: "Revision", Value: man.Revisions.Legacy}}
	if sel.Output != "" && man.Observation.Complete && man.Summary.Failures == 0 {
		rows = append(rows, render.Row{Key: "Wrote", Value: sel.Output})
	}
	if man.Summary.Excluded > 0 {
		rows = append(rows, render.Row{Key: "Dropped", Value: render.Comma(man.Summary.Excluded)})
	}
	if man.Summary.Failures > 0 {
		rows = append(rows, render.Row{Key: "Failed", Value: render.Comma(man.Summary.Failures)})
	}
	return rows
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
