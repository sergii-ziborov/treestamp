package policy

import (
	"strings"
	"testing"

	"github.com/sergii-ziborov/treestamp"
)

func TestNormalizeProfile(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ProfileRepo},
		{"repo", ProfileRepo},
		{"REPO-V1", ProfileRepo},
		{"artifact", ProfileArtifact},
		{"artifact-v2", ProfileArtifact},
		{"artifact-v1", ProfileArtifactV1},
	}
	for _, c := range cases {
		got, err := NormalizeProfile(c.in)
		if err != nil || got != c.want {
			t.Fatalf("%q → %q %v", c.in, got, err)
		}
	}
	if _, err := NormalizeProfile("hashdeep-v1"); err == nil {
		t.Fatal("unknown profile")
	}
	if !KnownProfile("") || !KnownProfile(ProfileArtifact) || KnownProfile("hashdeep-v1") {
		t.Fatal("known")
	}
	if DisplayProfile(ProfileArtifact) != "artifact" || DisplayProfile(ProfileRepo) != "repo" {
		t.Fatal("display")
	}
	ign := DisplayIgnore(treestamp.DefaultOptions().IgnoreFiles)
	if ign != ".gitignore, .ignore" || strings.Contains(ign, "weavatrix") {
		t.Fatalf("ignore %q", ign)
	}
}

func TestArtifactOptionsHashBinaries(t *testing.T) {
	sel := Select{Profile: "artifact", Format: "text", Color: "auto"}
	if err := sel.ApplyConfig(); err != nil {
		t.Fatal(err)
	}
	opts, err := sel.Options()
	if err != nil {
		t.Fatal(err)
	}
	if opts.DetectBinaryFiles || opts.MaxFileBytes != 0 || opts.IgnoreFiles != nil || !opts.HashFileContents || opts.StandardSkips || !opts.VCSSkips {
		t.Fatalf("%+v", opts)
	}
	snap := sel.Snapshot(opts)
	if snap.Profile != ProfileArtifact || !snap.NoIgnore {
		t.Fatalf("%+v", snap)
	}
	restored, err := OptionsFrom(snap)
	if err != nil || restored.DetectBinaryFiles || restored.MaxFileBytes != 0 || restored.IgnoreFiles != nil || restored.StandardSkips || !restored.VCSSkips {
		t.Fatalf("restore %+v %v", restored, err)
	}
}

func TestArtifactV1KeepsGeneratedSkips(t *testing.T) {
	opts, err := OptionsFrom(Snapshot{Profile: ProfileArtifactV1, HashContents: true, StandardSkips: true})
	if err != nil || !opts.StandardSkips || opts.VCSSkips {
		t.Fatalf("%+v %v", opts, err)
	}
}

func TestOptionsFromOldRepoKeepsBinaryDetect(t *testing.T) {
	for _, name := range []string{"", ProfileRepo} {
		opts, err := OptionsFrom(Snapshot{Profile: name, HashContents: true})
		if err != nil || !opts.DetectBinaryFiles {
			t.Fatalf("%q detect=%v %v", name, opts.DetectBinaryFiles, err)
		}
	}
}

func TestArtifactRejectsMetadataOnly(t *testing.T) {
	sel := Select{Profile: "artifact", MetadataOnly: true, Format: "text", Color: "auto"}
	if _, err := sel.Options(); err == nil {
		t.Fatal("expected error")
	}
}
