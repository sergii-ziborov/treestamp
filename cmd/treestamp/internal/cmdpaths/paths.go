package cmdpaths

import (
	"context"
	"path/filepath"

	"github.com/sergii-ziborov/treestamp"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/app"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/policy"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/render"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/status"
	"github.com/spf13/cobra"
)

func New(env *app.Env) *cobra.Command {
	var sel policy.Select
	var nul, absolute bool
	cmd := &cobra.Command{
		Use:   "paths [ROOT]",
		Short: "List selected paths without reading file bytes",
		Example: "  treestamp paths . --ext go\n" +
			"  treestamp paths . --ext go --json",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			return run(cmd.Context(), env, sel, root, nul, absolute)
		},
	}
	sel.Bind(cmd)
	cmd.Flags().BoolVar(&nul, "null", false, "separate names with a NUL (safe for spaces)")
	cmd.Flags().BoolVar(&absolute, "absolute", false, "print absolute paths")
	return cmd
}

func run(ctx context.Context, env *app.Env, sel policy.Select, root string, nul, absolute bool) error {
	if err := sel.ApplyConfig(); err != nil {
		return env.Fail(status.Usage, "config: %v", err)
	}
	if sel.FormatName() == "json" && nul {
		return env.Fail(status.Usage, "--json and --null cannot be combined")
	}
	opts, err := sel.Options()
	if err != nil {
		return env.Fail(status.Usage, "%v", err)
	}
	if sel.MetadataOnly {
		return env.Fail(status.Usage, "paths is already content-free; --metadata-only is invalid here")
	}
	paths, err := treestamp.ScanPathsWith(ctx, root, treestamp.Using(opts))
	if err != nil {
		return env.Fail(status.Impossible, "%v", err)
	}
	if absolute {
		var absErr error
		paths, absErr = absPaths(root, paths)
		if absErr != nil {
			return env.Fail(status.Impossible, "%v", absErr)
		}
	}
	if sel.FormatName() == "json" {
		return render.JSON(env.Out, map[string]any{"schema": "treestamp.paths/v1", "paths": paths})
	}
	return writeLines(env, paths, nul)
}

func absPaths(root string, paths []string) ([]string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Join(absRoot, filepath.FromSlash(p))
	}
	return out, nil
}

func writeLines(env *app.Env, paths []string, nul bool) error {
	for _, p := range paths {
		if nul {
			if _, err := env.Out.Write([]byte(p + "\x00")); err != nil {
				return env.Fail(status.Publish, "stdout: %v", err)
			}
			continue
		}
		if err := render.Line(env.Out, render.EscapeLine(p)); err != nil {
			return env.Fail(status.Publish, "stdout: %v", err)
		}
	}
	return nil
}
