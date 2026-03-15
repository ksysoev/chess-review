package chessreview

import "math"

// Classification represents the quality rating of a chess move.
type Classification int

const (
	// Book indicates the move is part of known opening theory (ECO database).
	// Book moves are not judged by engine evaluation — they represent memorised
	// theory and are excluded from accuracy calculations.
	Book Classification = iota
	// Brilliant indicates a material sacrifice that is the engine's top choice,
	// improves or maintains the position after the sacrifice, and was made when
	// the position was not already clearly winning. Corresponds to the "!!"
	// annotation in standard chess notation.
	Brilliant
	// Great indicates a critical turning-point move: one that swings the position
	// from losing to equal/winning, or from equal to clearly winning, while also
	// keeping the win-probability loss within the Excellent threshold (≤ 2%).
	// These moves are decisive contributions that change the expected outcome of
	// the game without throwing away the newly gained advantage.
	Great
	// Best indicates the move matches the engine's top choice.
	Best
	// Excellent indicates a near-optimal move with minimal win-probability loss (0–2%).
	Excellent
	// Good indicates a solid move with small win-probability loss (2–5%).
	Good
	// Inaccuracy indicates a suboptimal move with noticeable win-probability loss (5–10%).
	Inaccuracy
	// Mistake indicates a poor move with significant win-probability loss (10–20%).
	Mistake
	// Blunder indicates a very bad move with severe win-probability loss (> 20%).
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
	case Great:
		return "Great"
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

// missThreshold is the centipawn loss at which a move is classified as Miss.
// Derived from mateScoreSentinel (2/3 of the sentinel = 20 000) so that the
// two values stay in sync automatically if the sentinel ever changes.
const missThreshold = mateScoreSentinel * 2 / 3

// Win-probability loss thresholds for move classification.
// Based on the chess.com Expected Points Model: each threshold represents the
// upper bound of win-probability lost for that classification tier.
//
//	Excellent  – 0–2%  win-probability loss
//	Good       – 2–5%  win-probability loss
//	Inaccuracy – 5–10% win-probability loss
//	Mistake    – 10–20% win-probability loss
//	Blunder    – >20%  win-probability loss
const (
	excellentWinProbThreshold  = 0.02
	goodWinProbThreshold       = 0.05
	inaccuracyWinProbThreshold = 0.10
	mistakeWinProbThreshold    = 0.20
)

// brilliantWinningThreshold is the pre-move evaluation (in centipawns) above
// which a sacrifice is not annotated as Brilliant. When the position is already
// clearly winning (≥ +2.00 / 200 cp) a sacrifice is technique rather than a
// spectacular find.
const brilliantWinningThreshold = 200

// greatLosingThreshold is the win-probability below which a position is
// considered losing for the Great move classification. A move that rescues the
// position out of losing territory (below this value) into equal or winning
// territory qualifies as Great.
const greatLosingThreshold = 0.40

// greatWinningThreshold is the win-probability above which a position is
// considered winning for the Great move classification. A move that converts
// the position from equal territory into winning territory (above this value)
// qualifies as Great.
const greatWinningThreshold = 0.60

// winProb converts a centipawn evaluation to a win probability in [0, 1] using
// a logistic (sigmoid) function calibrated to chess engine evaluations.
// The formula is: 1 / (1 + exp(-cp / 400))
// At 0 cp (equal) → 0.50; at +400 cp → ~0.73; at −400 cp → ~0.27.
func winProb(cp int) float64 {
	return 1.0 / (1.0 + math.Exp(-float64(cp)/400.0))
}

// winProbLoss returns the win-probability lost by a move.
// scoreBefore is the evaluation before the move and scoreAfter is the evaluation
// after the move; both are expressed from the perspective of the player who made
// the move (the same reference frame). scoreAfter is therefore the negated
// engine score, not the raw engine output from the opponent's point of view.
// Returns 0 if the move improved or maintained the position.
func winProbLoss(scoreBefore, scoreAfter int) float64 {
	loss := winProb(scoreBefore) - winProb(scoreAfter)
	if loss < 0 {
		return 0
	}

	return loss
}

// ClassifyContext holds all inputs required to classify a single half-move.
// Using a struct keeps the Classify signature stable as new contextual fields
// are added in the future.
type ClassifyContext struct {
	// PlayedMove is the UCI move that was actually played (e.g. "e2e4").
	PlayedMove string
	// BestMove is the engine's top-recommended move at the configured depth.
	BestMove string
	// ScoreBefore is the centipawn evaluation immediately before the move, from
	// the perspective of the side to move.
	ScoreBefore int
	// ScoreAfter is the centipawn evaluation immediately after the move, from
	// the perspective of the side that just moved (same reference frame as
	// ScoreBefore — i.e. the engine score negated after the move).
	ScoreAfter int
	// ScoreBeforePrev is the same player's ScoreBefore from two half-moves ago
	// (i.e. the player's own ScoreBefore on their previous turn). It is used to
	// detect the "capitalise on opponent's blunder" variant of a Great move:
	// the player was in a losing or equal position on their last turn, but the
	// opponent's intervening blunder swung the game, and this move seizes the
	// newly won advantage. Only meaningful when HasPrev is true.
	ScoreBeforePrev int
	// HasPrev is true when ScoreBeforePrev is valid. It is false for the first
	// move of each colour (no prior turn to look back to).
	HasPrev bool
	// IsSacrifice is true when the move gives up material that the opponent can
	// immediately recapture, making it a candidate for a Brilliant annotation.
	IsSacrifice bool
	// IsBook is true when the move is part of known opening theory (ECO
	// database). Book moves are returned immediately as Book — they are not
	// judged by engine evaluation.
	IsBook bool
}

// Classify returns the move classification for the given context.
//
// Classification priority (highest to lowest):
//
//	Book       – move is in the ECO opening book (theory)
//	Brilliant  – sacrifice that is the engine's top choice, improves or maintains
//	             the position (ScoreAfter >= ScoreBefore), and not already clearly winning (< +2.00)
//	Great      – critical turning-point: losing→equal/winning, or equal→clearly winning,
//	             AND win-probability loss ≤ 2% (excellentWinProbThreshold).
//	             Checked against both the immediate ScoreBefore (1-ply) and, when
//	             HasPrev is true, the same player's ScoreBefore from two half-moves
//	             ago (2-ply lookback) to capture moves that capitalise on an
//	             opponent blunder.
//	Best       – played move equals engine best (and not a qualifying sacrifice/turning-point)
//	Miss       – move throws away a forced mate (cp loss ≥ 20 000); checked before win-prob tiers
//	             because sentinel-based cp loss can produce misleading win-probability values
//	Excellent  – 0–2%  win-probability loss
//	Good       – 2–5%  win-probability loss
//	Inaccuracy – 5–10% win-probability loss
//	Mistake    – 10–20% win-probability loss
//	Blunder    – >20%  win-probability loss
func Classify(ctx ClassifyContext) Classification {
	// Book moves take priority over all engine-based classifications.
	if ctx.IsBook {
		return Book
	}

	cpLossVal := ctx.ScoreBefore - ctx.ScoreAfter
	if cpLossVal < 0 {
		cpLossVal = 0
	}

	wpLoss := winProbLoss(ctx.ScoreBefore, ctx.ScoreAfter)

	// Brilliant: a material sacrifice that is also the engine's top choice,
	// improves or maintains the position (ScoreAfter >= ScoreBefore), and was
	// made when the position was not already clearly winning (< +2.00 / 200 cp).
	// All four conditions must hold simultaneously:
	//   1. IsSacrifice — the move gives up material the opponent can recapture
	//   2. PlayedMove == BestMove — the engine endorses it as the top choice
	//   3. ScoreAfter >= ScoreBefore — the position does not worsen after the sacrifice
	//   4. ScoreBefore < brilliantWinningThreshold — not already clearly winning
	if ctx.IsSacrifice && ctx.PlayedMove == ctx.BestMove && ctx.ScoreAfter >= ctx.ScoreBefore && ctx.ScoreBefore < brilliantWinningThreshold {
		return Brilliant
	}

	// Great: a critical turning-point move that changes the expected outcome.
	//
	// 1-ply check: the current position itself crosses a threshold.
	//   Case A: rescues from a losing position into equal or winning territory.
	//   Case B: converts an equal position into a clearly winning one.
	//
	// 2-ply lookback: the player was in a losing/equal position on their
	//   previous turn (ScoreBeforePrev), the opponent's intervening move
	//   happened to be a blunder, and this move seizes the resulting decisive
	//   advantage. The same threshold checks are applied but against the
	//   player's position two half-moves ago rather than one.
	wpBefore := winProb(ctx.ScoreBefore)
	wpAfter := winProb(ctx.ScoreAfter)

	isGreat := (wpBefore < greatLosingThreshold && wpAfter >= greatLosingThreshold) ||
		(wpBefore < greatWinningThreshold && wpAfter >= greatWinningThreshold)

	if !isGreat && ctx.HasPrev {
		wpPrev := winProb(ctx.ScoreBeforePrev)
		isGreat = (wpPrev < greatLosingThreshold && wpAfter >= greatLosingThreshold) ||
			(wpPrev < greatWinningThreshold && wpAfter >= greatWinningThreshold)
	}

	if isGreat && wpLoss <= excellentWinProbThreshold {
		return Great
	}

	if ctx.PlayedMove == ctx.BestMove {
		return Best
	}

	// Miss: move throws away a forced mate (sentinel-based cp loss).
	if cpLossVal >= missThreshold {
		return Miss
	}

	switch {
	case wpLoss <= excellentWinProbThreshold:
		return Excellent
	case wpLoss <= goodWinProbThreshold:
		return Good
	case wpLoss <= inaccuracyWinProbThreshold:
		return Inaccuracy
	case wpLoss <= mistakeWinProbThreshold:
		return Mistake
	default:
		return Blunder
	}
}
