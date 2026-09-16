package cmdroot

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/app"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/cmddiff"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/cmdexplain"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/cmdmeta"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/cmdpaths"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/cmdscan"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/cmdverify"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/status"
	"github.com/spf13/cobra"
)

func New(env *app.Env) *cobra.Command {
	root := &cobra.Command{
		Use:   "treestamp",
		Short: "Verifiable repository scanning",
		Long: `Treestamp selects a tree by policy, writes a portable manifest,
explains the decision, and verifies the selected files later.

This is not find, ripgrep, Git, or a backup tool.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.SetOut(env.Out)
	root.SetErr(env.Err)
	root.CompletionOptions.HiddenDefaultCmd = false
	root.AddCommand(
		cmdscan.New(env),
		cmdpaths.New(env),
		cmdexplain.New(env),
		cmddiff.New(env),
		cmdverify.New(env),
		cmdmeta.Version(env),
		cmdmeta.Doctor(env),
		cmdmeta.ConfigShow(env),
		cmdmeta.Schema(env),
	)
	return root
}

func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	env := &app.Env{In: stdin, Out: stdout, Err: stderr}
	root := New(env)
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := root.ExecuteContext(ctx)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return status.Timeout
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return status.Interrupted
	}
	if env.Code != 0 {
		return env.Code
	}
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return status.Usage
	}
	return status.OK
}
