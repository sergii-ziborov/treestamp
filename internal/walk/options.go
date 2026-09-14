package walk

// ErrorPolicy selects whether a local walk error stops the walker.
type ErrorPolicy int

const (
	ErrorContinue ErrorPolicy = iota
	ErrorAbort
)

// RootSymlinkPolicy controls whether the supplied root may itself be a symlink.
type RootSymlinkPolicy int

const (
	RootFollow RootSymlinkPolicy = iota
	RootReject
)

// WalkOptions is the serial traversal policy.
type WalkOptions struct {
	MinDepth          int
	MaxDepth          *int
	MaxOpen           int
	SameFileSystem    bool
	FollowLinks       bool
	CollectMetadata   bool
	ErrorPolicy       ErrorPolicy
	RootSymlinkPolicy RootSymlinkPolicy
}

// DefaultMaxOpen is the oracle default open-directory bound.
const DefaultMaxOpen = 64

// DefaultOptions returns the oracle walker defaults.
func DefaultOptions() WalkOptions {
	return WalkOptions{
		MinDepth:          0,
		MaxDepth:          nil,
		MaxOpen:           DefaultMaxOpen,
		SameFileSystem:    false,
		FollowLinks:       false,
		CollectMetadata:   false,
		ErrorPolicy:       ErrorContinue,
		RootSymlinkPolicy: RootFollow,
	}
}

// Normalize applies the oracle clamping rules.
func (o WalkOptions) Normalize() WalkOptions {
	if o.MaxOpen <= 0 {
		o.MaxOpen = 1
	}
	if o.MaxDepth != nil && o.MinDepth > *o.MaxDepth {
		o.MinDepth = *o.MaxDepth
	}
	return o
}

func (o WalkOptions) atOrBeyondMaxDepth(depth int) bool {
	return o.MaxDepth != nil && depth >= *o.MaxDepth
}
