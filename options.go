package treestamp

// Options is the intended scan configuration. Fields match the oracle defaults
// so later stages do not invent a different product. Scan does not read them yet.
type Options struct {
	MaxFileBytes            uint64
	IgnoreFiles             []string
	SkipHidden              bool
	StandardSkips           bool
	HashFileContents        bool
	DetectBinaryFiles       bool
	EvidenceComplete        bool
	CacheValidationFast     bool
	ContentValidationStrict bool
	ContentDiscoveryStream  bool
}

// DefaultOptions returns the oracle default scan policy.
//
// Whole-scan entry and byte limits stay disabled. Hidden skipping stays off.
// Ignore files stay .gitignore, .ignore, and .weavatrixignore.
// .treestampignore is not added here.
func DefaultOptions() Options {
	return Options{
		MaxFileBytes:            1_500_000,
		IgnoreFiles:             []string{".gitignore", ".ignore", ".weavatrixignore"},
		SkipHidden:              false,
		StandardSkips:           true,
		HashFileContents:        true,
		DetectBinaryFiles:       true,
		EvidenceComplete:        true,
		CacheValidationFast:     true,
		ContentValidationStrict: true,
		ContentDiscoveryStream:  true,
	}
}

// ScannerOption will configure NewScanner. The constructor still fails.
type ScannerOption func(*scannerConfig)

type scannerConfig struct {
	options          Options
	traversalWorkers int
	contentWorkers   int
}

// WithOptions replaces the default scan options.
func WithOptions(options Options) ScannerOption {
	return func(cfg *scannerConfig) {
		cfg.options = options
	}
}

// WithTraversalWorkers sets a traversal worker count. Zero means auto later.
func WithTraversalWorkers(n int) ScannerOption {
	return func(cfg *scannerConfig) {
		cfg.traversalWorkers = n
	}
}

// WithContentWorkers sets a content worker count. Zero means auto later.
func WithContentWorkers(n int) ScannerOption {
	return func(cfg *scannerConfig) {
		cfg.contentWorkers = n
	}
}
