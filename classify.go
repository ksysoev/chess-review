package chessreview

// Classification represents the quality rating of a chess move.
type Classification int

const (
	// Best indicates the move matches the engine's top choice.
	Best Classification = iota
	// Excellent indicates a near-optimal move with minimal centipawn loss (1–10 cp).
	Excellent
	// Good indicates a solid move with small centipawn loss (11–25 cp).
	Good
	// Inaccuracy indicates a suboptimal move with noticeable centipawn loss (26–100 cp).
	Inaccuracy
	// Mistake indicates a poor move with significant centipawn loss (101–300 cp).
	Mistake
	// Blunder indicates a very bad move with severe centipawn loss (> 300 cp).
	Blunder
	// Miss indicates a move that misses an immediate winning tactic (e.g. missed mate).
	Miss
)

// String returns a human-readable label for the classification.
func (c Classification) String() string {
	switch c {
	case Best:
		return "Best"
	case Excellent:
		return "Excellent"
	case Good:
		return "Good"
	case Inaccuracy:
		return "Inaccuracy"
	case Mistake:
		return "Mistake"
	case Blunder:
		return "Blunder"
	case Miss:
		return "Miss"
	default:
		return "Unknown"
	}
}

const (
	excellentThreshold  = 10
	goodThreshold       = 25
	inaccuracyThreshold = 100
	mistakeThreshold    = 300
)

// Classify returns the move classification given the centipawn loss and whether
// the played move equals the engine's best move.
//
// scoreDelta is the change in centipawns from the perspective of the side that
// just moved: positive means the position improved for that side, negative means
// the played move cost material/position. We work with the absolute loss value.
//
// Thresholds mirror chess.com's game review grading:
//
//	Best       – played move equals engine best
//	Excellent  – 1–10 cp loss
//	Good       – 11–25 cp loss
//	Inaccuracy – 26–100 cp loss
//	Mistake    – 101–300 cp loss
//	Blunder    – > 300 cp loss
func Classify(scoreDelta int, playedMove, bestMove string) Classification {
	if playedMove == bestMove {
		return Best
	}

	loss := -scoreDelta
	if loss < 0 {
		loss = 0
	}

	switch {
	case loss <= excellentThreshold:
		return Excellent
	case loss <= goodThreshold:
		return Good
	case loss <= inaccuracyThreshold:
		return Inaccuracy
	case loss <= mistakeThreshold:
		return Mistake
	default:
		return Blunder
	}
}
