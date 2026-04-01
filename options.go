package chessreview

import "fmt"

const (
	// DefaultDepth is the default Stockfish search depth used when no WithDepth
	// option is provided. It is exported so that callers (e.g. a CLI) can use it
	// as their own default without duplicating the value.
	DefaultDepth = 18
	// DefaultThreads is the default number of CPU threads Stockfish may use.
	// It is exported so that callers (e.g. a CLI) can use it as their own default.
	DefaultThreads = 1
	// DefaultHashMB is the default transposition table size in megabytes.
	// It is exported so that callers (e.g. a CLI) can use it as their own default.
	DefaultHashMB = 16
	// DefaultTopMoves is the default number of candidate moves (principal
	// variations) the engine evaluates per position. It is exported so that
	// callers (e.g. a CLI) can use it as their own default.
	DefaultTopMoves = 3
)

// config holds internal configuration for a Reviewer.
type config struct {
	depth    int
	threads  int
	hashMB   int
	topMoves int
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

// WithTopMoves sets the number of candidate moves (principal variations) the
// engine evaluates for each position. The result is available as
// MoveReview.TopMoves, ordered from best to worst. The default is 3.
func WithTopMoves(n int) Option {
	return func(c *config) {
		c.topMoves = n
	}
}

func defaultConfig() config {
	return config{
		depth:    DefaultDepth,
		threads:  DefaultThreads,
		hashMB:   DefaultHashMB,
		topMoves: DefaultTopMoves,
	}
}

// validate checks that all config fields have sensible values.
// Returns an error if any value is out of the acceptable range.
func (c *config) validate() error {
	if c.depth < 1 {
		return fmt.Errorf("invalid depth %d: must be >= 1", c.depth)
	}

	if c.threads < 1 {
		return fmt.Errorf("invalid threads %d: must be >= 1", c.threads)
	}

	if c.hashMB < 1 {
		return fmt.Errorf("invalid hash size %d MB: must be >= 1", c.hashMB)
	}

	if c.topMoves < 1 {
		return fmt.Errorf("invalid top moves %d: must be >= 1", c.topMoves)
	}

	return nil
}
