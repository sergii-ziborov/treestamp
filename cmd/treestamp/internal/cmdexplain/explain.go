package cmdexplain

import (
	"strconv"

	"github.com/sergii-ziborov/treestamp"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/app"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/policy"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/render"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/status"
	"github.com/spf13/cobra"
)

func New(env *app.Env) *cobra.Command {
	var sel policy.Select
	var root string
	cmd := &cobra.Command{
		Use:   "explain PATH",
		Short: "Explain why a path is selected or excluded",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(env, sel, root, args[0])
		},
	}
	sel.Bind(cmd)
	cmd.Flags().StringVar(&root, "root", ".", "scan root; the path is root-relative")
	return cmd
}

func run(env *app.Env, sel policy.Select, root, rel string) error {
	if err := sel.ApplyConfig(); err != nil {
		return env.Fail(status.Usage, "config: %v", err)
	}
	opts, err := sel.Options()
	if err != nil {
		return env.Fail(status.Usage, "%v", err)
	}
	why, err := treestamp.Explain(root, rel, treestamp.Using(opts))
	if err != nil {
		return env.Fail(status.Impossible, "%v", err)
	}
	doc := map[string]any{
		"schema": "treestamp.explain/v1", "relative": why.Relative, "outcome": why.Outcome,
		"reason": why.Reason, "source": why.Source, "pattern": why.Pattern, "line": why.Line,
		"checked": []string{"selection"}, "not_checked": []string{"content bytes", "binary detection"},
	}
	if sel.FormatName() == "json" {
		return render.JSON(env.Out, doc)
	}
	pal := render.Detect(env.Out, sel.Color)
	tone := "ok"
	if why.Outcome != "included" && why.Outcome != "include" {
		tone = "warn"
	}
	if err := render.WriteCard(env.Out, pal, render.Card{
		Status: why.Outcome, Detail: why.Relative, Tone: tone,
		Rows: []render.Row{
			{Key: "Reason", Value: why.Reason},
			{Key: "Source", Value: formatSource(why)},
			{Key: "Rule", Value: why.Pattern},
			{Key: "Checked", Value: "selection rules"},
			{Key: "Not checked", Value: "content bytes, binary detection"},
		},
	}); err != nil {
		return env.Fail(status.Publish, "stdout: %v", err)
	}
	return nil
}

func formatSource(why treestamp.PathExplanation) string {
	if why.Line > 0 {
		return why.Source + ":" + strconv.Itoa(why.Line)
	}
	return why.Source
}
