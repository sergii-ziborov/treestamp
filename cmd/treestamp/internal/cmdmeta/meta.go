package cmdmeta

import (
	"runtime"
	"strings"

	"github.com/sergii-ziborov/treestamp"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/app"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/policy"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/render"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/status"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/store"
	"github.com/spf13/cobra"
)

func Version(env *app.Env) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print CLI and library versions",
		RunE: func(cmd *cobra.Command, args []string) error {
			doc := map[string]any{
				"cli": policy.CLIVersion, "core": treestamp.Version,
				"schema": store.Schema, "profile": policy.Profile,
			}
			if asJSON {
				return render.JSON(env.Out, doc)
			}
			return render.WriteCard(env.Out, render.Detect(env.Out, "auto"), render.Card{
				Status: "treestamp " + policy.CLIVersion,
				Rows:   []render.Row{{Key: "Library", Value: treestamp.Version}},
			})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON instead of the human summary")
	return cmd
}

func Doctor(env *app.Env) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Report local CLI capabilities without scanning or networking",
		RunE: func(cmd *cobra.Command, args []string) error {
			pal := render.Detect(env.Out, "auto")
			if err := render.WriteCard(env.Out, pal, render.Card{
				Status: "Doctor", Detail: "no network, no source mutation", Tone: "ok",
				Rows: []render.Row{
					{Key: "CLI", Value: policy.CLIVersion},
					{Key: "Core", Value: treestamp.Version},
					{Key: "Schema", Value: store.Schema},
					{Key: "Go", Value: runtime.Version()},
					{Key: "OS", Value: runtime.GOOS + "/" + runtime.GOARCH},
					{Key: "CGO", Value: "intended 0"},
					{Key: "Commands", Value: "scan paths explain diff verify"},
					{Key: "Watch", Value: "not in this release"},
				},
			}); err != nil {
				return env.Fail(status.Publish, "stdout: %v", err)
			}
			return nil
		},
	}
}

func Config(env *app.Env) *cobra.Command {
	var sel policy.Select
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show the effective scan policy without walking the tree",
		RunE:  func(cmd *cobra.Command, args []string) error { return runConfig(env, sel) },
	}
	sel.BindPolicy(cmd)
	sel.BindJSON(cmd)
	show := &cobra.Command{
		Use:    "show",
		Hidden: true,
		RunE:   func(cmd *cobra.Command, args []string) error { return runConfig(env, sel) },
	}
	cmd.AddCommand(show)
	return cmd
}

func runConfig(env *app.Env, sel policy.Select) error {
	if err := sel.ApplyConfig(); err != nil {
		return env.Fail(status.Usage, "config: %v", err)
	}
	opts, err := sel.Options()
	if err != nil {
		return env.Fail(status.Usage, "%v", err)
	}
	snap := sel.Snapshot(opts)
	if sel.FormatName() == "json" {
		return render.JSON(env.Out, snap)
	}
	return render.WriteCard(env.Out, render.Detect(env.Out, sel.Color), render.Card{
		Status: "Policy",
		Rows: []render.Row{
			{Key: "Profile", Value: policy.DisplayProfile(snap.Profile)},
			{Key: "Extensions", Value: strings.Join(snap.Extensions, ", ")},
			{Key: "Scope", Value: strings.Join(snap.Scope, ", ")},
			{Key: "Exclude", Value: strings.Join(snap.Exclude, ", ")},
			{Key: "Ignore files", Value: policy.DisplayIgnore(snap.IgnoreFiles)},
			{Key: "Hash contents", Value: hashContentsRow(snap.HashContents)},
		},
	})
}

func hashContentsRow(v bool) string {
	if v {
		return ""
	}
	return "no"
}

func Schema(env *app.Env) *cobra.Command {
	return &cobra.Command{
		Use:   "schema [manifest]",
		Short: "Print supported wire schema names",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return render.JSON(env.Out, map[string]any{
				"manifest": store.Schema,
				"policy":   "treestamp.policy/v1",
				"explain":  "treestamp.explain/v1",
				"diff":     "treestamp.diff/v1",
				"verify":   "treestamp.verify/v1",
			})
		},
	}
}
