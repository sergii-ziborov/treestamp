package cmdmeta

import (
	"runtime"

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
			return render.Line(env.Out, "treestamp "+policy.CLIVersion+" (core "+treestamp.Version+")")
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable versions")
	return cmd
}

func Doctor(env *app.Env) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Report local CLI capabilities without scanning or networking",
		RunE: func(cmd *cobra.Command, args []string) error {
			pal := render.Detect(env.Out, "auto")
			if err := render.WriteCard(env.Out, pal, render.Card{
				Status: "DOCTOR", Detail: "no network, no source mutation", Tone: "ok",
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

func ConfigShow(env *app.Env) *cobra.Command {
	var sel policy.Select
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect effective policy without scanning",
	}
	show := &cobra.Command{
		Use:   "show",
		Short: "Print the effective scan policy",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := sel.ApplyConfig(); err != nil {
				return env.Fail(status.Usage, "config: %v", err)
			}
			opts, err := sel.Options()
			if err != nil {
				return env.Fail(status.Usage, "%v", err)
			}
			return render.JSON(env.Out, sel.Snapshot(opts))
		},
	}
	sel.Bind(show)
	cmd.AddCommand(show)
	return cmd
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
