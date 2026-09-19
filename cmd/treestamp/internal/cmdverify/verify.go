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
	Null     bool
	Color    string
}

func New(env *app.Env) *cobra.Command {
	req := request{}
	cmd := &cobra.Command{
		Use:   "verify SNAPSHOT",
		Short: "Re-scan a tree and compare it to a saved snapshot",
		Example: "  treestamp verify ./baselines/cli.tstamp.json --root ./cmd/treestamp\n" +
			"  treestamp verify ./baselines/cli.tstamp.json --root ./cmd/treestamp --null",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req.Manifest = args[0]
			return run(cmd.Context(), env, req)
		},
	}
	cmd.Flags().StringVar(&req.Root, "root", "", "folder to read (required; not taken from the snapshot)")
	cmd.Flags().BoolVar(&req.JSON, "json", false, "print JSON instead of the human summary")
	cmd.Flags().BoolVar(&req.Null, "null", false, "print every changed path, NUL-separated")
	cmd.Flags().StringVar(&req.Color, "color", "auto", "auto, always, or never")
	return cmd
}

func run(ctx context.Context, env *app.Env, req request) error {
	if req.JSON && req.Null {
		return env.Fail(status.Usage, "--json and --null cannot be combined")
	}
	if req.Root == "" {
		return env.Fail(status.Usage, "verify needs --root (the folder to read)")
	}
	base, err := store.Load(req.Manifest)
	if err != nil {
		return env.Fail(status.Impossible, "baseline: %v", err)
	}
	if !base.Observation.Complete {
		return env.Fail(status.Impossible, "snapshot is incomplete; scan again")
	}
	if base.Observation.Evidence != "sha256" {
		return env.Fail(status.Impossible, "snapshot has no content hashes; scan again without --metadata-only")
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
	return finish(env, current, delta, req)
}

func finish(env *app.Env, current *treestamp.ScanReport, delta treestamp.ScanDelta, req request) error {
	if !current.Complete && env.Code != status.Partial {
		env.Set(status.Partial)
	}
	doc := map[string]any{
		"schema": "treestamp.verify/v1", "target": "selected-content-and-policy", "scope": "tree",
		"added": names(delta.Added), "removed": names(delta.Removed), "changed": changed(delta.Modified),
		"renamed": renamed(delta.Renamed), "selection_inputs_changed": delta.SelectionInputsChanged,
		"policy_changed": delta.PolicyChanged, "complete": current.Complete, "cache_not_used": true,
	}
	if req.JSON {
		if err := render.JSON(env.Out, doc); err != nil {
			return env.Fail(status.Publish, "stdout: %v", err)
		}
	} else if req.Null {
		if err := render.WriteNull(env.Out, render.OfDelta(delta).Lines()); err != nil {
			return env.Fail(status.Publish, "stdout: %v", err)
		}
	} else if err := writeHuman(env, delta, current, req.Color); err != nil {
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

func writeHuman(env *app.Env, delta treestamp.ScanDelta, current *treestamp.ScanReport, color string) error {
	pal := render.Detect(env.Out, color)
	card := render.Card{Status: "Match", Detail: "Selected files match the snapshot", Tone: "ok"}
	if env.Code == status.Partial {
		card.Status, card.Detail, card.Tone = "Partial", "Walk did not finish", "warn"
	} else if !delta.IsEmpty() {
		card.Status, card.Detail, card.Tone = "Differ", "Selected files changed", "bad"
		view := render.OfDelta(delta)
		card.Rows = view.Rows()
		card.Items = view.Lines()
	}
	if card.Status == "Match" {
		card.Rows = []render.Row{{Key: "Selected", Value: render.Comma(len(current.Files))}}
	}
	return render.WriteCard(env.Out, pal, card)
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
