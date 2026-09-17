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
		Use:   "verify SNAPSHOT",
		Short: "Re-scan a tree and compare it to a saved snapshot",
		Example: "  treestamp verify ./baselines/cli.tstamp.json --root ./cmd/treestamp\n" +
			"  treestamp verify ./baselines/cli.tstamp.json --root ./cmd/treestamp --json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req.Manifest = args[0]
			return run(cmd.Context(), env, req)
		},
	}
	cmd.Flags().StringVar(&req.Root, "root", "", "folder to read (required; not taken from the snapshot)")
	cmd.Flags().BoolVar(&req.JSON, "json", false, "print JSON instead of the human summary")
	return cmd
}

func run(ctx context.Context, env *app.Env, req request) error {
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
	card := render.Card{Status: "Match", Detail: "Selected files match the snapshot", Tone: "ok"}
	if env.Code == status.Partial {
		card.Status, card.Detail, card.Tone = "Partial", "Walk did not finish", "warn"
	} else if !delta.IsEmpty() {
		card.Status, card.Detail, card.Tone = "Differ", "Selected files changed", "bad"
		card.Rows = deltaRows(delta)
		card.Items = deltaItems(delta)
	}
	if card.Status == "Match" {
		card.Rows = []render.Row{{Key: "Selected", Value: render.Comma(len(current.Files))}}
	}
	return render.WriteCard(env.Out, pal, card)
}

func deltaRows(delta treestamp.ScanDelta) []render.Row {
	return []render.Row{
		render.CountRow("Added", len(delta.Added)),
		render.CountRow("Removed", len(delta.Removed)),
		render.CountRow("Changed", len(delta.Modified)),
		render.CountRow("Renamed", len(delta.Renamed)),
	}
}

func deltaItems(delta treestamp.ScanDelta) []string {
	items := make([]string, 0, 8)
	for _, f := range delta.Added {
		items = append(items, "+ "+f.Relative)
	}
	for _, f := range delta.Removed {
		items = append(items, "- "+f.Relative)
	}
	for _, item := range delta.Modified {
		items = append(items, "~ "+item.Current.Relative)
	}
	for _, item := range delta.Renamed {
		items = append(items, item.Previous.Relative+" → "+item.Current.Relative)
	}
	return render.Preview(items, 8)
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
