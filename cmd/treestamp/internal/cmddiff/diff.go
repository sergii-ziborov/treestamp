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
	var asJSON, exitCode bool
	cmd := &cobra.Command{
		Use:   "diff BEFORE AFTER",
		Short: "Compare two saved manifests without opening the tree",
		Example: "  treestamp diff ./baselines/before.tstamp.json ./baselines/after.tstamp.json --exit-code\n" +
			"  treestamp diff ./baselines/before.tstamp.json ./baselines/after.tstamp.json --json",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(env, args[0], args[1], asJSON, exitCode)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable delta")
	cmd.Flags().BoolVar(&exitCode, "exit-code", false, "exit 1 when a completed comparison finds differences")
	return cmd
}

func run(env *app.Env, before, after string, asJSON, exitCode bool) error {
	left, err := store.Load(before)
	if err != nil {
		return env.Fail(status.Impossible, "before: %v", err)
	}
	right, err := store.Load(after)
	if err != nil {
		return env.Fail(status.Impossible, "after: %v", err)
	}
	if !left.Observation.Complete || !right.Observation.Complete {
		return env.Fail(status.Partial, "one or both manifests are incomplete")
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
	tone, title := "ok", "EQUAL"
	if !delta.IsEmpty() {
		tone, title = "warn", "DIFFER"
	}
	notes := []string{}
	if delta.SelectionInputsChanged {
		notes = append(notes, "Selection inputs changed; this is not a plain file deletion.")
	}
	if delta.PolicyChanged {
		notes = append(notes, "Effective policy changed.")
	}
	return render.WriteCard(env.Out, pal, render.Card{
		Status: title, Detail: "manifest comparison only", Tone: tone,
		Rows: []render.Row{
			{Key: "Added", Value: render.Comma(len(delta.Added))},
			{Key: "Removed", Value: render.Comma(len(delta.Removed))},
			{Key: "Changed", Value: render.Comma(len(delta.Modified))},
			{Key: "Renamed", Value: render.Comma(len(delta.Renamed))},
			{Key: "Unchanged", Value: render.Comma(int(delta.Unchanged))},
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
