package treestamp

import (
	"os"

	"github.com/sergii-ziborov/treestamp/internal/platform"
	"github.com/sergii-ziborov/treestamp/internal/walk"
)

type (
	ErrorPolicy       = walk.ErrorPolicy
	RootSymlinkPolicy = walk.RootSymlinkPolicy
	WalkOptions       = walk.WalkOptions
	WalkOperation     = walk.WalkOperation
	WalkSkipReason    = walk.WalkSkipReason
	WalkEntry         = walk.WalkEntry
	WalkError         = walk.WalkError
	FileVersion       = walk.FileVersion
	Walker            = walk.Walker
	WalkBuilder       = walk.Builder
	MultiWalker       = walk.MultiWalker
	FileIdentity      = platform.Identity
)

const (
	ErrorContinue          = walk.ErrorContinue
	ErrorAbort             = walk.ErrorAbort
	RootFollow             = walk.RootFollow
	RootReject             = walk.RootReject
	DefaultMaxOpen         = walk.DefaultMaxOpen
	OpCanonicalize         = walk.OpCanonicalize
	OpReadDirectory        = walk.OpReadDirectory
	OpReadEntry            = walk.OpReadEntry
	OpReadMetadata         = walk.OpReadMetadata
	OpScheduleWorker       = walk.OpScheduleWorker
	SkipNone               = walk.SkipNone
	SkipMaxDepth           = walk.SkipMaxDepth
	SkipFileSystemBoundary = walk.SkipFileSystemBoundary
	SkipPathEscape         = walk.SkipPathEscape
	SkipSymlinkLoop        = walk.SkipSymlinkLoop
)

// DefaultWalkOptions returns the oracle serial walker defaults.
func DefaultWalkOptions() WalkOptions {
	return walk.DefaultOptions()
}

// NewWalker starts a serial walker at root.
func NewWalker(root string) (*Walker, error) {
	return walk.New(root)
}

// NewWalkerWithOptions starts a serial walker with an explicit policy.
func NewWalkerWithOptions(root string, options WalkOptions) (*Walker, error) {
	return walk.NewWithOptions(root, options)
}

// NewWalkBuilder starts a serial multi-root builder.
func NewWalkBuilder(root string) *WalkBuilder {
	return walk.NewBuilder(root)
}

// CollectWalk drains a walker. It is a test and driver helper, not Scan.
func CollectWalk(next func() (*WalkEntry, error)) ([]*WalkEntry, error) {
	type puller struct{ fn func() (*WalkEntry, error) }
	return walk.Collect(adapter(next))
}

type adapter func() (*WalkEntry, error)

func (a adapter) Next() (*WalkEntry, error) { return a() }

// NameSort compares directory entries by native name.
func NameSort(a, b os.DirEntry) int {
	if a.Name() < b.Name() {
		return -1
	}
	if a.Name() > b.Name() {
		return 1
	}
	return 0
}
