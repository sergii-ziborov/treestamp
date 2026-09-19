package cmddiff

import (
	"github.com/sergii-ziborov/treestamp"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/app"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/render"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/status"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/store"
	"github.com/spf13/cobra"
)

func New(env *app.Env) *cobra.Command {
	var asJSON, exitCode, nul bool
	cmd := &cobra.Command{
		Use:   "diff BEFORE AFTER",
		Short: "Compare two snapshots without opening the tree",
		Example: "  treestamp diff ./baselines/before.tstamp.json ./baselines/after.tstamp.json --exit-code\n" +
			"  treestamp diff ./baselines/before.tstamp.json ./baselines/after.tstamp.json --null",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(env, args[0], args[1], asJSON, exitCode, nul)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON instead of the human summary")
	cmd.Flags().BoolVar(&nul, "null", false, "print every changed path, NUL-separated")
	cmd.Flags().BoolVar(&exitCode, "exit-code", false, "exit 1 when the snapshots differ")
	return cmd
}

func run(env *app.Env, before, after string, asJSON, exitCode, nul bool) error {
	if asJSON && nul {
		return env.Fail(status.Usage, "--json and --null cannot be combined")
	}
	left, err := store.Load(before)
	if err != nil {
		return env.Fail(status.Impossible, "before: %v", err)
	}
	right, err := store.Load(after)
	if err != nil {
		return env.Fail(status.Impossible, "after: %v", err)
	}
	if !left.Observation.Complete || !right.Observation.Complete {
		return env.Fail(status.Partial, "one or both snapshots are incomplete")
	}
	delta := treestamp.DeltaBetween(left.AsReport(), right.AsReport())
	doc := map[string]any{
		"schema":                   "treestamp.diff/v1",
		"added":                    names(delta.Added),
		"removed":                  names(delta.Removed),
		"content_changed":          modified(delta.Modified),
		"renamed":                  renamed(delta.Renamed),
		"selection_inputs_changed": delta.SelectionInputsChanged,
		"policy_changed":           delta.PolicyChanged,
		"observation_changed":      delta.ScanStateChanged,
		"quality":                  quality(delta.Quality),
	}
	if asJSON {
		if err := render.JSON(env.Out, doc); err != nil {
			return env.Fail(status.Publish, "stdout: %v", err)
		}
	} else if nul {
		if err := render.WriteNull(env.Out, render.OfDelta(delta).Lines()); err != nil {
			return env.Fail(status.Publish, "stdout: %v", err)
		}
	} else if err := writeHuman(env, delta); err != nil {
		return env.Fail(status.Publish, "stdout: %v", err)
	}
	if exitCode && !delta.IsEmpty() {
		env.Set(status.Differ)
	}
	return nil
}

func writeHuman(env *app.Env, delta treestamp.ScanDelta) error {
	pal := render.Detect(env.Out, "auto")
	card := render.Card{Status: "Equal", Detail: "The two snapshots match", Tone: "ok"}
	if !delta.IsEmpty() {
		view := render.OfDelta(delta)
		card.Status, card.Detail, card.Tone = "Differ", "The two snapshots differ", "warn"
		card.Rows = view.Rows()
		if delta.PolicyChanged {
			card.Rows = append(card.Rows, render.Row{Key: "Policy", Value: "changed"})
		}
		card.Items = view.Lines()
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

func modified(items []treestamp.ModifiedFile) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Current.Relative)
	}
	return out
}

func renamed(items []treestamp.RenamedFile) []map[string]string {
	out := make([]map[string]string, 0, len(items))
	for _, item := range items {
		kind := "inferred_by_content"
		out = append(out, map[string]string{"from": item.Previous.Relative, "to": item.Current.Relative, "evidence": kind})
	}
	return out
}

func quality(q treestamp.DeltaQuality) string {
	switch q {
	case treestamp.DeltaContentHash:
		return "content"
	case treestamp.DeltaMetadata:
		return "metadata"
	default:
		return "partial"
	}
}
