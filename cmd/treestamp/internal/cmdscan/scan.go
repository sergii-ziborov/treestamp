package cmdscan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sergii-ziborov/treestamp"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/app"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/policy"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/render"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/status"
	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/store"
	"github.com/spf13/cobra"
)

func New(env *app.Env) *cobra.Command {
	var sel policy.Select
	cmd := &cobra.Command{
		Use:   "scan [ROOT]",
		Short: "Hash selected files and print a summary",
		Long:  "Scan ROOT (default .) and print what was selected. --output must be outside ROOT.",
		Example: "  treestamp scan ./cmd/treestamp --ext go --output ./baselines/cli.tstamp.json\n" +
			"  treestamp scan . --ext go --output ../repo.tstamp.json",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			return run(cmd.Context(), env, sel, root)
		},
	}
	sel.Bind(cmd)
	return cmd
}

func run(ctx context.Context, env *app.Env, sel policy.Select, root string) error {
	if err := sel.ApplyConfig(); err != nil {
		return env.Fail(status.Usage, "config: %v", err)
	}
	opts, err := sel.Options()
	if err != nil {
		return env.Fail(status.Usage, "%v", err)
	}
	if sel.Output != "" {
		inside, err := store.InsideRoot(root, sel.Output)
		if err != nil {
			return env.Fail(status.Impossible, "%v", err)
		}
		if inside {
			return env.Fail(status.Usage, "refusing --output inside scan root; write the manifest outside the tree")
		}
	}
	if sel.FormatName() == "ndjson" {
		return runNDJSON(ctx, env, sel, root, opts)
	}
	rep, err := treestamp.ScanWith(ctx, root, treestamp.Using(opts))
	if errors.Is(err, treestamp.ErrPartial) {
		env.Set(status.Partial)
	} else if err != nil {
		return env.Fail(mapScan(err), "%v", err)
	}
	if rep == nil {
		return env.Fail(status.Impossible, "scan produced no report")
	}
	man := store.FromReport(rep, sel.Snapshot(opts))
	return publish(env, sel, man, err)
}

func publish(env *app.Env, sel policy.Select, man store.Manifest, scanErr error) error {
	format := sel.FormatName()
	var payload []byte
	if sel.Output != "" || format == "json" {
		encoded, err := encodeManifest(man)
		if err != nil {
			return env.Fail(status.Publish, "encode: %v", err)
		}
		payload = encoded
	}
	if sel.Output != "" {
		if !man.Observation.Complete || env.Code == status.Partial {
			fmt.Fprintln(env.Err, "output baseline was not replaced")
		} else if err := store.WriteAtomic(sel.Output, payload); err != nil {
			return env.Fail(status.Publish, "write output: %v", err)
		}
	}
	if err := writeScan(env, sel, man, format, payload); err != nil {
		return err
	}
	if scanErr != nil && env.Code == 0 {
		env.Set(status.Partial)
	}
	return nil
}

func encodeManifest(man store.Manifest) ([]byte, error) {
	payload, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(bytes.TrimRight(payload, "\n"), '\n'), nil
}

func writeScan(env *app.Env, sel policy.Select, man store.Manifest, format string, payload []byte) error {
	switch format {
	case "json":
		if _, err := env.Out.Write(payload); err != nil {
			return env.Fail(status.Publish, "stdout: %v", err)
		}
	case "text":
		if err := writeHuman(env, sel, man); err != nil {
			return env.Fail(status.Publish, "stdout: %v", err)
		}
	default:
		return env.Fail(status.Usage, "unknown format %q", format)
	}
	return nil
}

func writeHuman(env *app.Env, sel policy.Select, man store.Manifest) error {
	pal := render.Detect(env.Out, firstNonEmpty(env.Color, sel.Color))
	tone, title := "ok", "Complete"
	if !man.Observation.Complete || man.Summary.Failures > 0 {
		tone, title = "warn", "Partial"
		if env.Code == 0 {
			env.Set(status.Partial)
		}
	}
	card := render.Card{
		Status: title, Detail: scanDetail(man), Tone: tone,
		Lead: scanNames(man), Rows: scanRows(sel, man),
		Items: render.Preview(man.Dropped, maxScanNames),
	}
	return render.WriteCard(env.Out, pal, card)
}

const maxScanNames = 20

func scanNames(man store.Manifest) []string {
	if len(man.Files) == 0 {
		return nil
	}
	out := make([]string, 0, len(man.Files))
	for _, file := range man.Files {
		out = append(out, file.Relative)
	}
	return render.Preview(out, maxScanNames)
}

func scanDetail(man store.Manifest) string {
	n := man.Summary.Selected
	if n == 0 {
		return "no files selected"
	}
	unit := render.Comma(n) + " files"
	if n == 1 {
		unit = "1 file"
	}
	if man.Summary.Hashed == n {
		return unit + " selected and hashed"
	}
	if man.Summary.Hashed == 0 {
		return unit + " selected (not hashed)"
	}
	return unit + " selected, " + render.Comma(man.Summary.Hashed) + " hashed"
}

func scanRows(sel policy.Select, man store.Manifest) []render.Row {
	var rows []render.Row
	if sel.Output != "" {
		rows = append(rows, render.Row{Key: "Revision", Value: man.Revisions.Legacy})
	}
	if man.Policy.Profile == policy.ProfileArtifact || man.Policy.Profile == policy.ProfileArtifactV1 {
		rows = append(rows, render.Row{Key: "Profile", Value: policy.DisplayProfile(man.Policy.Profile)})
	}
	if sel.Output != "" && man.Observation.Complete && man.Summary.Failures == 0 {
		rows = append(rows, render.Row{Key: "Wrote", Value: sel.Output})
	}
	if man.Summary.Excluded > 0 {
		rows = append(rows, render.Row{Key: "Dropped", Value: render.Comma(man.Summary.Excluded)})
	}
	if man.Summary.Failures > 0 {
		rows = append(rows, render.Row{Key: "Failed", Value: render.Comma(man.Summary.Failures)})
	}
	return rows
}

func mapScan(err error) int {
	var te *treestamp.Error
	if errors.As(err, &te) && te.Code == treestamp.CodeTimeout {
		return status.Timeout
	}
	return status.Impossible
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
