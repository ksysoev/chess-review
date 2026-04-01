// Package chessreview provides chess game analysis using the Stockfish engine.
package chessreview

import (
	"math"
)

// numClassifications is the total number of Classification values.
// Derived from the last iota constant so it stays in sync automatically.
const numClassifications = int(Miss) + 1

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

// numPhases is the total number of GamePhase values.
// Derived from the last iota constant so it stays in sync automatically.
const numPhases = int(Endgame) + 1

// phaseOpeningMax is the last full-move number considered to be the opening.
const phaseOpeningMax = 10

// phaseMiddlegameMax is the last full-move number considered to be the middlegame.
const phaseMiddlegameMax = 25

// accuracyCoeffA, accuracyCoeffB, and accuracyCoeffC are the constants for the
// Lichess per-move accuracy formula:
//
//	accuracy = A * exp(B * (wpBefore - wpAfter)) + C
//
// where wpBefore and wpAfter are win percentages on a [0, 100] scale.
// An additional +1 "uncertainty bonus" is applied.
// Clamped to the range [0, 100].
//
// Source: https://github.com/lichess-org/lila/blob/master/modules/analyse/src/main/scala/AccuracyPercent.scala
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

// gameRatingMidpoint is the rating corresponding to 50% accuracy.
const gameRatingMidpoint = 850.0

// gameRatingSlope controls how steeply accuracy maps to rating in the logistic
// formula. Calibrated so that 80% → ~1700, 90% → ~2200, 95% → ~2660.
const gameRatingSlope = 615.0

// initialPositionCP is the centipawn evaluation of the starting position, used
// as the first element of the per-color win-percent list. Lichess uses Cp.initial = 15.
const initialPositionCP = 15

// slidingWindowMinSize and slidingWindowMaxSize bound the number of positions
// in each volatility window. The window size is numMoves/10, clamped to [2, 8].
const (
	slidingWindowMinSize = 2
	slidingWindowMaxSize = 8
)

// volatilityWeightMin and volatilityWeightMax clamp the standard deviation of
// each volatility window to produce meaningful weights.
const (
	volatilityWeightMin = 0.5
	volatilityWeightMax = 12.0
)

// PlayerSummary holds per-player aggregated statistics derived from a game review.
type PlayerSummary struct {
	// ClassificationCounts holds the count of each move classification.
	// Index with a Classification constant (e.g. ClassificationCounts[Brilliant]).
	ClassificationCounts [numClassifications]int
	// PhaseAccuracy holds accuracy percentages for each game phase.
	// Index with a GamePhase constant (e.g. PhaseAccuracy[Opening]).
	// A value of math.NaN() means the player had no moves in that phase.
	PhaseAccuracy [numPhases]float64
	// Accuracy is the overall accuracy percentage (0–100), computed using the
	// Lichess sliding-window volatility-weighted algorithm.
	Accuracy float64
	// GameRating is an estimated Elo-like rating derived from the accuracy score
	// using a logistic mapping.
	GameRating int
}

// GameSummary holds the aggregated summary for both players in a game.
type GameSummary struct {
	WhitePlayer  string
	BlackPlayer  string
	OpeningCode  string
	OpeningTitle string
	White        PlayerSummary
	Black        PlayerSummary
}

// Summarize builds a GameSummary from a slice of MoveReviews, player names,
// and the ECO opening code and title detected from the game's moves.
// It computes per-color classification counts, overall accuracy (using the
// Lichess sliding-window algorithm), estimated game rating (logistic mapping),
// and per-phase accuracy (harmonic mean of per-move accuracies) for each player.
// Book moves are excluded from accuracy calculations because they represent
// memorised theory rather than the player's own decisions.
func Summarize(reviews []MoveReview, whiteName, blackName, openingCode, openingTitle string) GameSummary {
	type playerAccum struct {
		// Per-move accuracies grouped by game phase.
		phaseAccuracies [numPhases][]float64
		// All per-move accuracies (for overall game accuracy).
		allAccuracies []float64
		// Volatility weights, one per entry in allAccuracies.
		// Derived from the shared all-ply win% series via computePlyWeights.
		allWeights []float64
		counts     [numClassifications]int
	}

	// Build a single combined win-percent series (White's perspective) that
	// covers every ply in order, matching the Lichess algorithm exactly.
	// The series starts with the initial-position evaluation and appends one
	// entry per move (the win% after the move, from White's perspective).
	allWPs := make([]float64, 0, len(reviews)+1)
	allWPs = append(allWPs, winPercent(initialPositionCP))

	for _, r := range reviews {
		switch r.Color {
		case colorWhite:
			allWPs = append(allWPs, winPercent(r.ScoreAfter))
		case colorBlack:
			allWPs = append(allWPs, 100.0-winPercent(r.ScoreAfter))
		}
	}

	// Pre-compute one volatility weight per ply from the shared series.
	plyWeights := computePlyWeights(allWPs, len(reviews))

	var white, black playerAccum

	for i, r := range reviews {
		var acc *playerAccum

		switch r.Color {
		case colorWhite:
			acc = &white
		case colorBlack:
			acc = &black
		default:
			// Skip moves with an unrecognised color so they do not silently
			// skew one player's statistics.
			continue
		}

		// Classification counts — always counted, even for Book moves.
		if r.Classification >= 0 && int(r.Classification) < numClassifications {
			acc.counts[r.Classification]++
		}

		// Book moves are excluded from accuracy calculations.
		if r.Classification == Book {
			continue
		}

		// Exclude mate-sentinel-based moves from accuracy so a single
		// missed-mate doesn't collapse accuracy to near zero.
		cpLossVal := r.ScoreBefore - r.ScoreAfter
		if cpLossVal >= missThreshold {
			continue
		}

		// Compute win percentages from the played side's perspective.
		wpBefore := winPercent(r.ScoreBefore)
		wpAfter := winPercent(r.ScoreAfter)

		ma := moveAccuracy(wpBefore, wpAfter)

		phase := phaseOf(r.MoveNumber)
		acc.phaseAccuracies[phase] = append(acc.phaseAccuracies[phase], ma)
		acc.allAccuracies = append(acc.allAccuracies, ma)

		// Attach the pre-computed weight for this ply.
		if i < len(plyWeights) {
			acc.allWeights = append(acc.allWeights, plyWeights[i])
		}
	}

	return GameSummary{
		WhitePlayer:  whiteName,
		BlackPlayer:  blackName,
		OpeningCode:  openingCode,
		OpeningTitle: openingTitle,
		White:        buildPlayerSummary(&white.counts, white.phaseAccuracies, white.allAccuracies, white.allWeights),
		Black:        buildPlayerSummary(&black.counts, black.phaseAccuracies, black.allAccuracies, black.allWeights),
	}
}

// computePlyWeights builds one volatility weight per ply from the combined
// all-ply win-percent series. It mirrors the Lichess sliding-window logic:
// window size = clamp(n/10, 2, 8), padded for early plies.
func computePlyWeights(allWPs []float64, n int) []float64 {
	if n == 0 {
		return nil
	}

	windowSize := n / 10
	if windowSize < slidingWindowMinSize {
		windowSize = slidingWindowMinSize
	} else if windowSize > slidingWindowMaxSize {
		windowSize = slidingWindowMaxSize
	}

	windows := buildSlidingWindows(allWPs, windowSize, n)
	weights := make([]float64, n)

	for i, w := range windows {
		sd := standardDeviation(w)
		weights[i] = math.Max(volatilityWeightMin, math.Min(volatilityWeightMax, sd))
	}

	return weights
}

// buildPlayerSummary assembles a PlayerSummary from accumulated data.
func buildPlayerSummary(
	counts *[numClassifications]int,
	phaseAccuracies [numPhases][]float64,
	allAccuracies []float64,
	allWeights []float64,
) PlayerSummary {
	var phaseAcc [numPhases]float64

	for i := range phaseAccuracies {
		if len(phaseAccuracies[i]) == 0 {
			phaseAcc[i] = math.NaN()
		} else {
			phaseAcc[i] = harmonicMean(phaseAccuracies[i])
		}
	}

	var overallAcc float64
	if len(allAccuracies) == 0 {
		overallAcc = math.NaN()
	} else {
		overallAcc = gameAccuracy(allAccuracies, allWeights)
	}

	return PlayerSummary{
		ClassificationCounts: *counts,
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

// moveAccuracy computes the per-move accuracy from win percentages before and
// after a move, using the Lichess formula. Both wpBefore and wpAfter are on the
// [0, 100] scale (from the played side's perspective).
//
// If the move improved the position (wpAfter >= wpBefore), accuracy is 100.
// Otherwise:
//
//	accuracy = 103.1668 * exp(-0.04354 * (wpBefore - wpAfter)) - 3.1669 + 1
//
// The +1 is the Lichess "uncertainty bonus". The result is clamped to [0, 100].
//
// Source: https://github.com/lichess-org/lila/blob/master/modules/analyse/src/main/scala/AccuracyPercent.scala
func moveAccuracy(wpBefore, wpAfter float64) float64 {
	if wpAfter >= wpBefore {
		return 100.0
	}

	diff := wpBefore - wpAfter
	acc := accuracyCoeffA*math.Exp(accuracyCoeffB*diff) + accuracyCoeffC + 1.0

	return math.Max(0, math.Min(100, acc))
}

// gameAccuracy computes the overall game accuracy using the Lichess
// sliding-window volatility-weighted algorithm.
//
// It expects pre-computed per-move volatility weights (one per accuracy entry)
// produced by computePlyWeights. The overall accuracy is the average of:
//   - the volatility-weighted arithmetic mean of per-move accuracies, and
//   - the harmonic mean of per-move accuracies.
//
// Source: https://github.com/lichess-org/lila/blob/master/modules/analyse/src/main/scala/AccuracyPercent.scala
func gameAccuracy(accuracies, weights []float64) float64 {
	n := len(accuracies)
	if n == 0 {
		return math.NaN()
	}

	if n == 1 {
		return accuracies[0]
	}

	wm := weightedMean(accuracies, weights)
	hm := harmonicMean(accuracies)

	return (wm + hm) / 2.0
}

// buildSlidingWindows creates n sliding windows from the values slice, each of
// the given windowSize. The first (windowSize - 2) windows are padded copies of
// the first real window (i.e. values[0:windowSize]).
//
// This matches the Lichess implementation: the first few moves get the same
// volatility weight as the first real window because there isn't enough data
// for distinct windows yet.
func buildSlidingWindows(values []float64, windowSize, n int) [][]float64 {
	windows := make([][]float64, n)

	// Build the first real window.
	firstWindow := safeSlice(values, 0, windowSize)

	// Padding: the first (windowSize - 2) entries use the first window.
	padCount := windowSize - 2
	if padCount < 0 {
		padCount = 0
	}

	for i := range n {
		if i < padCount {
			windows[i] = firstWindow
		} else {
			start := i - padCount
			windows[i] = safeSlice(values, start, start+windowSize)
		}
	}

	return windows
}

// safeSlice returns values[start:end], clamping start and end to [0, len(values)].
func safeSlice(values []float64, start, end int) []float64 {
	if start < 0 {
		start = 0
	}

	if start > len(values) {
		start = len(values)
	}

	if end > len(values) {
		end = len(values)
	}

	if start >= end {
		return nil
	}

	return values[start:end]
}

// standardDeviation computes the population standard deviation of values.
// Returns 0 if len(values) < 2.
func standardDeviation(values []float64) float64 {
	n := len(values)
	if n < 2 {
		return 0
	}

	var sum float64
	for _, v := range values {
		sum += v
	}

	mean := sum / float64(n)

	var variance float64

	for _, v := range values {
		d := v - mean
		variance += d * d
	}

	variance /= float64(n)

	return math.Sqrt(variance)
}

// weightedMean computes the weighted arithmetic mean of values with the given
// weights. If all weights are zero, it falls back to a simple arithmetic mean.
func weightedMean(values, weights []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	var sumWV, sumW float64

	for i, v := range values {
		w := 1.0
		if i < len(weights) {
			w = weights[i]
		}

		sumWV += w * v
		sumW += w
	}

	if sumW == 0 {
		// Fallback to simple arithmetic mean.
		var sum float64
		for _, v := range values {
			sum += v
		}

		return sum / float64(len(values))
	}

	return sumWV / sumW
}

// harmonicMean computes the harmonic mean of values. Values <= 0 are replaced
// with a small epsilon (0.01) to avoid division by zero while still penalising
// very low accuracy moves heavily.
func harmonicMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	const epsilon = 0.01

	var sumReciprocal float64

	for _, v := range values {
		if v <= 0 {
			v = epsilon
		}

		sumReciprocal += 1.0 / v
	}

	return float64(len(values)) / sumReciprocal
}

// calcGameRating estimates an Elo-like game rating from an accuracy percentage
// using a logistic mapping:
//
//	rating = 850 + 615 * ln(accuracy / (100 - accuracy))
//
// Calibration points: 50% → 850, 70% → 1370, 80% → 1700, 90% → 2200, 95% → 2660.
// Clamped to [gameRatingMin, gameRatingMax].
// Returns gameRatingMin when accuracy is NaN.
func calcGameRating(accuracy float64) int {
	if math.IsNaN(accuracy) {
		return gameRatingMin
	}

	// Clamp accuracy to avoid log(0) or log(negative).
	acc := math.Max(0.5, math.Min(99.5, accuracy))

	rating := gameRatingMidpoint + gameRatingSlope*math.Log(acc/(100.0-acc))
	rating = math.Max(float64(gameRatingMin), math.Min(float64(gameRatingMax), rating))

	return int(math.Round(rating))
}
