package policy

import (
	"fmt"
	"strings"

	"github.com/sergii-ziborov/treestamp"
)

const (
	ProfileRepo     = "repo-v1"
	ProfileArtifact = "artifact-v1"
)

func NormalizeProfile(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "repo", ProfileRepo:
		return ProfileRepo, nil
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
	if name == ProfileArtifact {
		return "artifact"
	}
	return "repo"
}

func applyArtifact(opts *treestamp.Options) {
	opts.DetectBinaryFiles = false
	opts.MaxFileBytes = 0
	opts.IgnoreFiles = nil
	opts.HashFileContents = true
}
