package chessreview

// Classification represents the quality rating of a chess move.
type Classification int

const (
	// Book indicates the move is part of known opening theory (ECO database).
	// Book moves are not judged by engine evaluation — they represent memorised
	// theory and are excluded from accuracy calculations.
	Book Classification = iota
	// Brilliant indicates a material sacrifice that leads to an excellent or better
	// position, provided the position was not already overwhelmingly winning before
	// the move. Corresponds to the "!!" annotation in standard chess notation.
	Brilliant
	// Best indicates the move matches the engine's top choice.
	Best
	// Excellent indicates a near-optimal move with minimal centipawn loss (0–10 cp).
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
	case Book:
		return "Book"
	case Brilliant:
		return "Brilliant"
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

// mateScoreSentinel is the centipawn value used to represent a forced mate.
// Positive means the side to move has a forced mate; negative means they are
// being mated. The magnitude is chosen to be far outside any real centipawn
// range while still leaving room for delta arithmetic without overflow.
// It is also used by normalizeScore in review.go.
const mateScoreSentinel = 30_000

const (
	excellentThreshold  = 10
	goodThreshold       = 25
	inaccuracyThreshold = 100
	mistakeThreshold    = 300
	// missThreshold is the centipawn loss at which a move is classified as Miss.
	// Derived from mateScoreSentinel (2/3 of the sentinel = 20 000) so that the
	// two values stay in sync automatically if the sentinel ever changes.
	missThreshold = mateScoreSentinel * 2 / 3
	// brilliantWinningThreshold is the pre-move evaluation above which a sacrifice
	// is not annotated as Brilliant. When the position is already overwhelmingly
	// winning (≥ +9.00, queen-equivalent advantage) a sacrifice is merely
	// technique rather than a spectacular find.
	brilliantWinningThreshold = 900
)

// Classify returns the move classification given the centipawn delta, the
// pre-move evaluation, and contextual flags.
//
// isBook reports whether the move is part of known opening theory (ECO
// database). Book moves are returned immediately as Book — they are not judged
// by engine evaluation.
//
// scoreDelta is the change in centipawns from the perspective of the side that
// just moved: positive means the position improved for that side, negative means
// the played move cost material/position. We work with the absolute loss value.
//
// scoreBefore is the centipawn evaluation immediately before the move, from the
// perspective of the side to move. It is used to suppress Brilliant annotations
// when the position was already overwhelmingly winning (≥ +9.00 / 900 cp).
//
// isSacrifice reports whether the move gives up material that the opponent can
// immediately recapture, making it a candidate for a Brilliant annotation.
//
// Thresholds used for move classification grading:
//
//	Book       – move is in the ECO opening book (theory)
//	Brilliant  – sacrifice with ≤10 cp loss when not already clearly winning
//	Best       – played move equals engine best (and not a qualifying sacrifice)
//	Excellent  – 0–10 cp loss
//	Good       – 11–25 cp loss
//	Inaccuracy – 26–100 cp loss
//	Mistake    – 101–300 cp loss
//	Blunder    – > 300 cp loss
//	Miss       – move throws away a forced mate (sentinel loss ≥ 20000 cp)
func Classify(scoreDelta, scoreBefore int, playedMove, bestMove string, isSacrifice, isBook bool) Classification {
	// Book moves take priority over all engine-based classifications.
	if isBook {
		return Book
	}

	loss := -scoreDelta
	if loss < 0 {
		loss = 0
	}

	// Brilliant: a sacrifice that keeps the evaluation in excellent range, but
	// only when the position was not already overwhelmingly winning beforehand.
	if isSacrifice && loss <= excellentThreshold && scoreBefore < brilliantWinningThreshold {
		return Brilliant
	}

	if playedMove == bestMove {
		return Best
	}

	switch {
	case loss >= missThreshold:
		return Miss
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
