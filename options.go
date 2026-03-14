package chessreview

const (
	defaultDepth   = 18
	defaultThreads = 1
	defaultHashMB  = 16
)

// config holds internal configuration for a Reviewer.
type config struct {
	depth   int
	threads int
	hashMB  int
}

// Option is a functional option for configuring a Reviewer.
type Option func(*config)

// WithDepth sets the search depth for engine analysis.
// Higher values produce stronger but slower analysis.
// The default depth is 18.
func WithDepth(depth int) Option {
	return func(c *config) {
		c.depth = depth
	}
}

// WithThreads sets the number of CPU threads the engine may use.
// The default is 1.
func WithThreads(threads int) Option {
	return func(c *config) {
		c.threads = threads
	}
}

// WithHash sets the transposition table size in megabytes.
// Larger values can improve analysis quality at the cost of memory.
// The default is 16 MB.
func WithHash(mb int) Option {
	return func(c *config) {
		c.hashMB = mb
	}
}

func defaultConfig() config {
	return config{
		depth:   defaultDepth,
		threads: defaultThreads,
		hashMB:  defaultHashMB,
	}
}
