// Package chessreview provides chess game analysis using the Stockfish engine.
package chessreview

import (
	"math"
)

// numClassifications is the total number of Classification values.
// It must stay in sync with the iota constants in classify.go.
const numClassifications = 8

// GamePhase represents a named phase of a chess game based on move number.
type GamePhase int

const (
	// Opening covers moves 1–10.
	Opening GamePhase = iota
	// Middlegame covers moves 11–25.
	Middlegame
	// Endgame covers moves 26 and beyond.
	Endgame
)

// phaseOpeningMax is the last full-move number considered to be the opening.
const phaseOpeningMax = 10

// phaseMiddlegameMax is the last full-move number considered to be the middlegame.
const phaseMiddlegameMax = 25

// accuracyCoeffA, accuracyCoeffB, accuracyCoeffC are the constants for the
// chess.com accuracy formula:
//
//	accuracy = A * exp(B * avgCPL) + C
//
// Clamped to the range [0, 100].
const (
	accuracyCoeffA = 103.1668
	accuracyCoeffB = -0.04354
	accuracyCoeffC = -3.1669
)

// gameRatingMin and gameRatingMax clamp the estimated game rating.
const (
	gameRatingMin = 100
	gameRatingMax = 3000
)

// gameRatingScale and gameRatingOffset map accuracy (0–100) to an estimated
// Elo-like rating using the linear formula:
//
//	rating = accuracy * gameRatingScale - gameRatingOffset
const (
	gameRatingScale  = 30.0
	gameRatingOffset = 300
)

// PlayerSummary holds per-player aggregated statistics derived from a game review.
type PlayerSummary struct {
	// ClassificationCounts holds the count of each move classification.
	// Index with a Classification constant (e.g. ClassificationCounts[Brilliant]).
	ClassificationCounts [numClassifications]int
	// PhaseAccuracy holds accuracy percentages for each game phase.
	// Index with a GamePhase constant (e.g. PhaseAccuracy[Opening]).
	// A value of math.NaN() means the player had no moves in that phase.
	PhaseAccuracy [3]float64
	// Accuracy is the overall accuracy percentage (0–100), computed using the
	// chess.com formula from the player's average centipawn loss.
	Accuracy float64
	// GameRating is an estimated Elo-like rating derived from the accuracy score.
	GameRating int
}

// GameSummary holds the aggregated summary for both players in a game.
type GameSummary struct {
	WhitePlayer string
	BlackPlayer string
	White       PlayerSummary
	Black       PlayerSummary
}

// Summarize builds a GameSummary from a slice of MoveReviews and player names.
// It computes per-color classification counts, overall accuracy, estimated game
// rating, and per-phase accuracy for each player.
func Summarize(reviews []MoveReview, whiteName, blackName string) GameSummary {
	type playerAccum struct {
		counts [numClassifications]int
		phases [3]phaseAccum
		total  phaseAccum
	}

	var white, black playerAccum

	for _, r := range reviews {
		var acc *playerAccum
		if r.Color == "white" {
			acc = &white
		} else {
			acc = &black
		}

		// Classification counts.
		if int(r.Classification) < numClassifications {
			acc.counts[r.Classification]++
		}

		// Centipawn loss: cap at 0 and exclude mate sentinel values so that
		// sentinel arithmetic doesn't distort the average.
		loss := cpLoss(r.ScoreDelta)
		if loss >= 0 {
			phase := phaseOf(r.MoveNumber)
			acc.phases[phase].totalLoss += float64(loss)
			acc.phases[phase].count++
			acc.total.totalLoss += float64(loss)
			acc.total.count++
		}
	}

	return GameSummary{
		WhitePlayer: whiteName,
		BlackPlayer: blackName,
		White:       buildPlayerSummary(white.counts, white.phases, white.total),
		Black:       buildPlayerSummary(black.counts, black.phases, black.total),
	}
}

// phaseAccum accumulates centipawn loss data for a single game phase.
type phaseAccum struct {
	totalLoss float64
	count     int
}

// buildPlayerSummary assembles a PlayerSummary from accumulated data.
func buildPlayerSummary(
	counts [numClassifications]int,
	phases [3]phaseAccum,
	total phaseAccum,
) PlayerSummary {
	var phaseAcc [3]float64

	for i := range phases {
		if phases[i].count == 0 {
			phaseAcc[i] = math.NaN()
		} else {
			phaseAcc[i] = calcAccuracy(phases[i].totalLoss / float64(phases[i].count))
		}
	}

	var overallAcc float64
	if total.count == 0 {
		overallAcc = math.NaN()
	} else {
		overallAcc = calcAccuracy(total.totalLoss / float64(total.count))
	}

	return PlayerSummary{
		ClassificationCounts: counts,
		PhaseAccuracy:        phaseAcc,
		Accuracy:             overallAcc,
		GameRating:           calcGameRating(overallAcc),
	}
}

// phaseOf returns the GamePhase for a given full-move number.
func phaseOf(moveNumber int) GamePhase {
	switch {
	case moveNumber <= phaseOpeningMax:
		return Opening
	case moveNumber <= phaseMiddlegameMax:
		return Middlegame
	default:
		return Endgame
	}
}

// cpLoss returns the centipawn loss for a move delta, clamped to [0, ∞).
// Sentinel values (≥ missThreshold in absolute magnitude) are excluded by
// returning -1, signalling the caller to skip this move from CPL averages.
func cpLoss(scoreDelta int) int {
	loss := -scoreDelta
	if loss < 0 {
		// The move improved the position — zero loss.
		return 0
	}

	// Exclude mate-sentinel-based deltas from the CPL average so that a single
	// missed-mate doesn't collapse the accuracy to near zero.
	if loss >= missThreshold {
		return -1
	}

	return loss
}

// calcAccuracy converts an average centipawn loss to an accuracy percentage
// using the chess.com formula, clamped to [0, 100].
//
//	accuracy = 103.1668 * exp(-0.04354 * avgCPL) - 3.1669
func calcAccuracy(avgCPL float64) float64 {
	acc := accuracyCoeffA*math.Exp(accuracyCoeffB*avgCPL) + accuracyCoeffC
	return math.Max(0, math.Min(100, acc))
}

// calcGameRating estimates an Elo-like game rating from an accuracy percentage.
// Returns gameRatingMin when accuracy is NaN.
func calcGameRating(accuracy float64) int {
	if math.IsNaN(accuracy) {
		return gameRatingMin
	}

	rating := accuracy*gameRatingScale - gameRatingOffset
	rating = math.Max(gameRatingMin, math.Min(gameRatingMax, rating))

	return int(math.Round(rating))
}
