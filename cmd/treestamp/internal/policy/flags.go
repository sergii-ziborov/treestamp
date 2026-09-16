package policy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sergii-ziborov/treestamp"
	"github.com/spf13/cobra"
)

const (
	CLIVersion = "0.1.0"
	Profile    = "repo-v1"
)

type Select struct {
	Exts, Scope, Exclude []string
	NoIgnore             bool
	MetadataOnly         bool
	Jobs                 int
	Config               string
	Format               string
	JSON                 bool
	Output               string
	Color                string
	Quiet                bool
}

type Snapshot struct {
	Profile       string                   `json:"profile"`
	Extensions    []string                 `json:"extensions,omitempty"`
	Scope         []string                 `json:"scope,omitempty"`
	Exclude       []string                 `json:"exclude,omitempty"`
	NoIgnore      bool                     `json:"no_ignore,omitempty"`
	HashContents  bool                     `json:"hash_contents"`
	MaxFileBytes  uint64                   `json:"max_file_bytes"`
	StandardSkips bool                     `json:"standard_skips"`
	IgnoreFiles   []string                 `json:"ignore_files,omitempty"`
	Descriptor    treestamp.ScanDescriptor `json:"descriptor"`
}

type fileConfig struct {
	Schema       string   `json:"schema"`
	Extensions   []string `json:"extensions"`
	Scope        []string `json:"scope"`
	Exclude      []string `json:"exclude"`
	NoIgnore     bool     `json:"no_ignore"`
	MaxFileBytes *uint64  `json:"max_file_bytes"`
}

func (s *Select) Bind(cmd *cobra.Command) {
	cmd.Flags().StringSliceVar(&s.Exts, "ext", nil, "keep files with these extensions")
	cmd.Flags().StringSliceVar(&s.Scope, "scope", nil, "narrow selection; does not override ignore rules")
	cmd.Flags().StringSliceVar(&s.Exclude, "exclude", nil, "subtract matching relative paths")
	cmd.Flags().BoolVar(&s.NoIgnore, "no-ignore", false, "disable ignore files; keeps standard skips and limits")
	cmd.Flags().BoolVar(&s.MetadataOnly, "metadata-only", false, "do not hash file contents")
	cmd.Flags().IntVar(&s.Jobs, "jobs", 0, "traversal workers (0 = library default)")
	cmd.Flags().StringVar(&s.Config, "config", "", "explicit versioned JSON policy file")
	cmd.Flags().StringVar(&s.Format, "format", "text", "text|json|ndjson")
	cmd.Flags().BoolVar(&s.JSON, "json", false, "same as --format json")
	cmd.Flags().StringVar(&s.Output, "output", "", "write the result document outside the scan root")
	cmd.Flags().StringVar(&s.Color, "color", "auto", "auto|always|never")
	cmd.Flags().BoolVar(&s.Quiet, "quiet", false, "hide human-only extras")
}

func (s *Select) ApplyConfig() error {
	if s.Config == "" {
		return nil
	}
	data, err := os.ReadFile(s.Config)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var cfg fileConfig
	if err := dec.Decode(&cfg); err != nil {
		return err
	}
	if cfg.Schema != "" && cfg.Schema != "treestamp.policy/v1" {
		return fmt.Errorf("unsupported config schema %q", cfg.Schema)
	}
	if len(s.Exts) == 0 {
		s.Exts = cfg.Extensions
	}
	if len(s.Scope) == 0 {
		s.Scope = cfg.Scope
	}
	if len(s.Exclude) == 0 {
		s.Exclude = cfg.Exclude
	}
	s.NoIgnore = s.NoIgnore || cfg.NoIgnore
	return nil
}

func (s Select) FormatName() string {
	if s.JSON || strings.EqualFold(s.Format, "json") {
		return "json"
	}
	return strings.ToLower(s.Format)
}

func (s Select) Options() (treestamp.Options, error) {
	opts := treestamp.DefaultOptions()
	opts.Extensions = append([]string(nil), s.Exts...)
	opts.Filters.LocationInclude = append([]string(nil), s.Scope...)
	opts.Filters.LocationExclude = append([]string(nil), s.Exclude...)
	if s.NoIgnore {
		opts.IgnoreFiles = nil
	}
	if s.MetadataOnly {
		opts.HashFileContents = false
	}
	if s.Jobs > 0 {
		opts.TraversalWorkers = s.Jobs
	}
	return opts, nil
}

func (s Select) Snapshot(opts treestamp.Options) Snapshot {
	return Snapshot{
		Profile: Profile, Extensions: opts.Extensions, Scope: s.Scope, Exclude: s.Exclude,
		NoIgnore: s.NoIgnore, HashContents: opts.HashFileContents, MaxFileBytes: opts.MaxFileBytes,
		StandardSkips: opts.StandardSkips, IgnoreFiles: opts.IgnoreFiles,
		Descriptor: treestamp.DescriptorFromOptions(opts),
	}
}

func OptionsFrom(snap Snapshot) (treestamp.Options, error) {
	opts := treestamp.DefaultOptions()
	opts.Extensions = append([]string(nil), snap.Extensions...)
	opts.Filters.LocationInclude = append([]string(nil), snap.Scope...)
	opts.Filters.LocationExclude = append([]string(nil), snap.Exclude...)
	opts.HashFileContents = snap.HashContents
	opts.MaxFileBytes = snap.MaxFileBytes
	opts.StandardSkips = snap.StandardSkips
	if snap.NoIgnore {
		opts.IgnoreFiles = nil
	} else if len(snap.IgnoreFiles) > 0 {
		opts.IgnoreFiles = append([]string(nil), snap.IgnoreFiles...)
	}
	return opts, nil
}
