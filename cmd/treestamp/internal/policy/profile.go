package policy

import (
	"fmt"
	"strings"

	"github.com/sergii-ziborov/treestamp"
)

const (
	ProfileRepo       = "repo-v1"
	ProfileArtifactV1 = "artifact-v1"
	ProfileArtifact   = "artifact-v2"
)

func NormalizeProfile(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "repo", ProfileRepo:
		return ProfileRepo, nil
	case ProfileArtifactV1:
		return ProfileArtifactV1, nil
	case "artifact", ProfileArtifact:
		return ProfileArtifact, nil
	default:
		return "", fmt.Errorf("unknown profile %q", name)
	}
}

func KnownProfile(name string) bool {
	_, err := NormalizeProfile(name)
	return err == nil
}

func DisplayProfile(name string) string {
	if name == ProfileArtifact || name == ProfileArtifactV1 {
		return "artifact"
	}
	return "repo"
}

func DisplayIgnore(files []string) string {
	if sameStrings(files, treestamp.DefaultOptions().IgnoreFiles) {
		return ".gitignore, .ignore"
	}
	return strings.Join(files, ", ")
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func applyArtifact(opts *treestamp.Options) {
	applyArtifactV2(opts)
}

func applyArtifactV1(opts *treestamp.Options) {
	opts.DetectBinaryFiles = false
	opts.MaxFileBytes = 0
	opts.IgnoreFiles = nil
	opts.HashFileContents = true
	opts.StandardSkips = true
	opts.VCSSkips = false
}

func applyArtifactV2(opts *treestamp.Options) {
	opts.DetectBinaryFiles = false
	opts.MaxFileBytes = 0
	opts.IgnoreFiles = nil
	opts.HashFileContents = true
	opts.StandardSkips = false
	opts.VCSSkips = true
}
