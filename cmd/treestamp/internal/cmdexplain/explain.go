package cmdexplain

import (
	"os"
	"path/filepath"
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
		Short: "Say why a path was kept or dropped",
		Example: "  treestamp explain skip.txt --root . --ext go\n" +
			"  treestamp explain cmd/treestamp/main.go --root . --json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(env, sel, root, args[0])
		},
	}
	sel.BindPolicy(cmd)
	sel.BindJSON(cmd)
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
	title, tone := explainTitle(why.Outcome)
	rows := explainRows(why)
	if title == "Kept" && missingOnDisk(root, rel) {
		title = "Would keep"
		rows = append(rows, render.Row{Key: "Why", Value: "not on disk"})
	}
	if err := render.WriteCard(env.Out, pal, render.Card{
		Status: title, Detail: why.Relative, Tone: tone, Rows: rows,
	}); err != nil {
		return env.Fail(status.Publish, "stdout: %v", err)
	}
	return nil
}

func missingOnDisk(root, rel string) bool {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	return os.IsNotExist(err)
}

func explainTitle(outcome string) (string, string) {
	switch outcome {
	case "excluded":
		return "Dropped", "warn"
	case "traverse":
		return "Directory", "ok"
	default:
		return "Kept", "ok"
	}
}

func explainRows(why treestamp.PathExplanation) []render.Row {
	rows := []render.Row{
		{Key: "Rule", Value: why.Pattern},
		{Key: "File", Value: formatSource(why)},
	}
	if why.Reason != "" && why.Reason != "ignore_rule" {
		rows = append(rows, render.Row{Key: "Why", Value: explainWhy(why.Reason)})
	}
	return rows
}

func explainWhy(reason string) string {
	switch reason {
	case "unselected":
		return "outside --scope or --ext"
	default:
		return reason
	}
}

func formatSource(why treestamp.PathExplanation) string {
	if why.Line > 0 {
		return why.Source + ":" + strconv.Itoa(why.Line)
	}
	return why.Source
}
