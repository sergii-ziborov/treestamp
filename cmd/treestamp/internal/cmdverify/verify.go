package cmdverify

import (
	"context"
	"errors"

	"github.com/sergii-ziborov/treestamp"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/app"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/policy"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/render"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/status"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/store"
	"github.com/spf13/cobra"
)

type request struct {
	Manifest string
	Root     string
	JSON     bool
}

func New(env *app.Env) *cobra.Command {
	req := request{}
	cmd := &cobra.Command{
		Use:   "verify MANIFEST",
		Short: "Re-scan the selected tree against a saved baseline",
		Example: "  treestamp verify ./baselines/cli.tstamp.json --root ./cmd/treestamp\n" +
			"  treestamp verify ./baselines/cli.tstamp.json --root ./cmd/treestamp --json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req.Manifest = args[0]
			return run(cmd.Context(), env, req)
		},
	}
	cmd.Flags().StringVar(&req.Root, "root", "", "tree to read; required, never taken from the manifest")
	cmd.Flags().BoolVar(&req.JSON, "json", false, "machine-readable verification")
	return cmd
}

func run(ctx context.Context, env *app.Env, req request) error {
	if req.Root == "" {
		return env.Fail(status.Usage, "verify requires --root")
	}
	base, err := store.Load(req.Manifest)
	if err != nil {
		return env.Fail(status.Impossible, "baseline: %v", err)
	}
	if !base.Observation.Complete {
		return env.Fail(status.Impossible, "incomplete baseline cannot be verified")
	}
	if base.Observation.Evidence != "sha256" {
		return env.Fail(status.Impossible, "baseline lacks content hashes; refusing to call metadata a content verify")
	}
	opts, err := policy.OptionsFrom(base.Policy)
	if err != nil {
		return env.Fail(status.Impossible, "%v", err)
	}
	current, err := treestamp.ScanWith(ctx, req.Root, treestamp.Using(opts))
	if errors.Is(err, treestamp.ErrPartial) {
		env.Set(status.Partial)
	} else if err != nil {
		return env.Fail(status.Impossible, "%v", err)
	}
	if current == nil {
		return env.Fail(status.Impossible, "verify produced no report")
	}
	prev := base.AsReport()
	prev.Root = current.Root
	delta := treestamp.DeltaBetween(prev, current)
	return finish(env, current, delta, req.JSON)
}

func finish(env *app.Env, current *treestamp.ScanReport, delta treestamp.ScanDelta, asJSON bool) error {
	if !current.Complete && env.Code != status.Partial {
		env.Set(status.Partial)
	}
	doc := map[string]any{
		"schema": "treestamp.verify/v1", "target": "selected-content-and-policy", "scope": "tree",
		"added": names(delta.Added), "removed": names(delta.Removed), "changed": changed(delta.Modified),
		"renamed": renamed(delta.Renamed), "selection_inputs_changed": delta.SelectionInputsChanged,
		"policy_changed": delta.PolicyChanged, "complete": current.Complete, "cache_not_used": true,
	}
	if asJSON {
		if err := render.JSON(env.Out, doc); err != nil {
			return env.Fail(status.Publish, "stdout: %v", err)
		}
	} else if err := writeHuman(env, delta, current); err != nil {
		return env.Fail(status.Publish, "stdout: %v", err)
	}
	if env.Code == status.Partial {
		return nil
	}
	if !delta.IsEmpty() {
		env.Set(status.Differ)
	}
	return nil
}

func writeHuman(env *app.Env, delta treestamp.ScanDelta, current *treestamp.ScanReport) error {
	pal := render.Detect(env.Out, "auto")
	tone, title := "ok", "MATCH"
	if env.Code == status.Partial {
		tone, title = "warn", "PARTIAL"
	} else if !delta.IsEmpty() {
		tone, title = "bad", "DIFFER"
	}
	notes := []string{"Fast cache was not used as content proof."}
	for _, item := range delta.Renamed {
		notes = append(notes, item.Previous.Relative+" -> "+item.Current.Relative)
	}
	return render.WriteCard(env.Out, pal, render.Card{
		Status: title, Detail: "selected-content-and-policy · tree", Tone: tone,
		Rows: []render.Row{
			{Key: "Added", Value: render.Comma(len(delta.Added))},
			{Key: "Removed", Value: render.Comma(len(delta.Removed))},
			{Key: "Changed", Value: render.Comma(len(delta.Modified))},
			{Key: "Renamed", Value: render.Comma(len(delta.Renamed))},
			{Key: "Selected now", Value: render.Comma(len(current.Files))},
		},
		Notes: notes,
	})
}

func names(files []treestamp.ScannedFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Relative)
	}
	return out
}

func changed(items []treestamp.ModifiedFile) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Current.Relative)
	}
	return out
}

func renamed(items []treestamp.RenamedFile) []map[string]string {
	out := make([]map[string]string, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]string{
			"from": item.Previous.Relative, "to": item.Current.Relative, "evidence": "inferred_by_content",
		})
	}
	return out
}
