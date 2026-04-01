package chessreview

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

// -----------------------------------------------------------------------
// phaseOf
// -----------------------------------------------------------------------

func TestPhaseOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		moveNumber int
		want       GamePhase
	}{
		{name: "move 1 is opening", moveNumber: 1, want: Opening},
		{name: "move 10 is opening", moveNumber: 10, want: Opening},
		{name: "move 11 is middlegame", moveNumber: 11, want: Middlegame},
		{name: "move 25 is middlegame", moveNumber: 25, want: Middlegame},
		{name: "move 26 is endgame", moveNumber: 26, want: Endgame},
		{name: "move 50 is endgame", moveNumber: 50, want: Endgame},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, phaseOf(tt.moveNumber))
		})
	}
}

// -----------------------------------------------------------------------
// moveAccuracy
// -----------------------------------------------------------------------

func TestMoveAccuracy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wpBefore float64
		wpAfter  float64
		wantMin  float64
		wantMax  float64
	}{
		{
			name:     "position improved → 100",
			wpBefore: 50.0, wpAfter: 60.0,
			wantMin: 100.0, wantMax: 100.0,
		},
		{
			name:     "position unchanged → 100",
			wpBefore: 50.0, wpAfter: 50.0,
			wantMin: 100.0, wantMax: 100.0,
		},
		{
			name: "small loss (~5 wp diff) → high accuracy",
			// winPercent(0) = 50, winPercent(-30) ≈ 47.24; diff ≈ 2.76.
			wpBefore: 50.0, wpAfter: 47.24,
			wantMin: 85.0, wantMax: 100.0,
		},
		{
			name: "moderate loss (~11 wp diff) → mid accuracy",
			// winPercent(0) = 50, winPercent(-60) ≈ 44.50; diff ≈ 5.5.
			wpBefore: 50.0, wpAfter: 44.5,
			wantMin: 70.0, wantMax: 85.0,
		},
		{
			name: "large loss (~25 wp diff) → low accuracy",
			// winPercent(0) = 50, winPercent(-300) ≈ 24.9; diff ≈ 25.1.
			wpBefore: 50.0, wpAfter: 24.9,
			wantMin: 25.0, wantMax: 45.0,
		},
		{
			name:     "massive loss near 100 wp diff → near zero",
			wpBefore: 97.0, wpAfter: 3.0,
			wantMin: 0.0, wantMax: 5.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := moveAccuracy(tt.wpBefore, tt.wpAfter)
			assert.GreaterOrEqual(t, got, tt.wantMin)
			assert.LessOrEqual(t, got, tt.wantMax)
		})
	}
}

func TestMoveAccuracy_NeverExceedsBounds(t *testing.T) {
	t.Parallel()

	for diff := 0.0; diff <= 100; diff += 1 {
		got := moveAccuracy(50+diff/2, 50-diff/2)
		assert.GreaterOrEqual(t, got, 0.0, "accuracy below 0 at diff=%.0f", diff)
		assert.LessOrEqual(t, got, 100.0, "accuracy above 100 at diff=%.0f", diff)
	}
}

// -----------------------------------------------------------------------
// standardDeviation
// -----------------------------------------------------------------------

func TestStandardDeviation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []float64
		want   float64
		delta  float64
	}{
		{name: "empty returns 0", values: nil, want: 0, delta: 0},
		{name: "single value returns 0", values: []float64{5}, want: 0, delta: 0},
		{name: "identical values returns 0", values: []float64{3, 3, 3}, want: 0, delta: 1e-9},
		// Population stddev of [2, 4, 4, 4, 5, 5, 7, 9] = 2.0.
		{name: "known dataset", values: []float64{2, 4, 4, 4, 5, 5, 7, 9}, want: 2.0, delta: 0.01},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.InDelta(t, tt.want, standardDeviation(tt.values), tt.delta)
		})
	}
}

// -----------------------------------------------------------------------
// weightedMean
// -----------------------------------------------------------------------

func TestWeightedMean(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		values  []float64
		weights []float64
		want    float64
		delta   float64
	}{
		{name: "empty returns 0", values: nil, weights: nil, want: 0, delta: 0},
		{name: "equal weights → arithmetic mean", values: []float64{10, 20, 30}, weights: []float64{1, 1, 1}, want: 20, delta: 1e-9},
		{name: "weighted towards first", values: []float64{10, 20}, weights: []float64{3, 1}, want: 12.5, delta: 1e-9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.InDelta(t, tt.want, weightedMean(tt.values, tt.weights), tt.delta)
		})
	}
}

// -----------------------------------------------------------------------
// harmonicMean
// -----------------------------------------------------------------------

func TestHarmonicMean(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []float64
		want   float64
		delta  float64
	}{
		{name: "empty returns 0", values: nil, want: 0, delta: 0},
		{name: "single value", values: []float64{5}, want: 5, delta: 1e-9},
		// Harmonic mean of [1, 4] = 2/(1/1 + 1/4) = 2/1.25 = 1.6.
		{name: "two values", values: []float64{1, 4}, want: 1.6, delta: 1e-9},
		// Harmonic mean of [100, 100, 100] = 100.
		{name: "identical values", values: []float64{100, 100, 100}, want: 100, delta: 1e-9},
		// Zero values use epsilon=0.01 → heavily penalised.
		{name: "zero value handled with epsilon", values: []float64{0, 100}, want: 0, delta: 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.InDelta(t, tt.want, harmonicMean(tt.values), tt.delta)
		})
	}
}

// -----------------------------------------------------------------------
// calcGameRating
// -----------------------------------------------------------------------

func TestCalcGameRating(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		accuracy float64
		wantMin  int
		wantMax  int
	}{
		{name: "NaN returns min", accuracy: math.NaN(), wantMin: gameRatingMin, wantMax: gameRatingMin},
		{name: "very low accuracy returns min", accuracy: 1, wantMin: gameRatingMin, wantMax: gameRatingMin},
		// Logistic: 50% → 850.
		{name: "50% accuracy → ~850", accuracy: 50, wantMin: 840, wantMax: 860},
		// Logistic: 80% → ~1700.
		{name: "80% accuracy → ~1700", accuracy: 80, wantMin: 1680, wantMax: 1720},
		// Logistic: 90% → ~2200.
		{name: "90% accuracy → ~2200", accuracy: 90, wantMin: 2180, wantMax: 2220},
		// Logistic: 95% → ~2660.
		{name: "95% accuracy → ~2660", accuracy: 95, wantMin: 2640, wantMax: 2680},
		// Very high accuracy capped at gameRatingMax.
		{name: "99.5% accuracy capped at max", accuracy: 99.5, wantMin: 2900, wantMax: gameRatingMax},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := calcGameRating(tt.accuracy)
			assert.GreaterOrEqual(t, got, tt.wantMin)
			assert.LessOrEqual(t, got, tt.wantMax)
		})
	}
}

// -----------------------------------------------------------------------
// Summarize
// -----------------------------------------------------------------------

func TestSummarize_PlayerNames(t *testing.T) {
	t.Parallel()

	reviews := []MoveReview{
		{Color: "white", MoveNumber: 1, Classification: Best, ScoreBefore: 15, ScoreAfter: 15, ScoreDelta: 0},
		{Color: "black", MoveNumber: 1, Classification: Good, ScoreBefore: 15, ScoreAfter: 0, ScoreDelta: -15},
	}

	summary := Summarize(reviews, "Alice", "Bob", "", "")

	assert.Equal(t, "Alice", summary.WhitePlayer)
	assert.Equal(t, "Bob", summary.BlackPlayer)
}

func TestSummarize_ClassificationCounts(t *testing.T) {
	t.Parallel()

	reviews := []MoveReview{
		{Color: "white", MoveNumber: 1, Classification: Best, ScoreBefore: 15, ScoreAfter: 15, ScoreDelta: 0},
		{Color: "white", MoveNumber: 2, Classification: Brilliant, ScoreBefore: 50, ScoreAfter: 55, ScoreDelta: 5},
		{Color: "white", MoveNumber: 3, Classification: Inaccuracy, ScoreBefore: 50, ScoreAfter: -10, ScoreDelta: -60},
		{Color: "black", MoveNumber: 1, Classification: Blunder, ScoreBefore: 50, ScoreAfter: -350, ScoreDelta: -400},
		{Color: "black", MoveNumber: 2, Classification: Blunder, ScoreBefore: 50, ScoreAfter: -300, ScoreDelta: -350},
	}

	summary := Summarize(reviews, "", "", "", "")

	assert.Equal(t, 1, summary.White.ClassificationCounts[Best])
	assert.Equal(t, 1, summary.White.ClassificationCounts[Brilliant])
	assert.Equal(t, 1, summary.White.ClassificationCounts[Inaccuracy])
	assert.Equal(t, 0, summary.White.ClassificationCounts[Blunder])

	assert.Equal(t, 2, summary.Black.ClassificationCounts[Blunder])
	assert.Equal(t, 0, summary.Black.ClassificationCounts[Best])
}

func TestSummarize_AccuracyPerfectPlay(t *testing.T) {
	t.Parallel()

	// All best moves with zero delta → accuracy should be near 100.
	reviews := []MoveReview{
		{Color: "white", MoveNumber: 1, Classification: Best, ScoreBefore: 15, ScoreAfter: 15, ScoreDelta: 0},
		{Color: "white", MoveNumber: 2, Classification: Best, ScoreBefore: 15, ScoreAfter: 15, ScoreDelta: 0},
		{Color: "white", MoveNumber: 3, Classification: Best, ScoreBefore: 15, ScoreAfter: 15, ScoreDelta: 0},
	}

	summary := Summarize(reviews, "", "", "", "")

	assert.InDelta(t, 100.0, summary.White.Accuracy, 1.0)
}

func TestSummarize_AccuracyBadPlay(t *testing.T) {
	t.Parallel()

	// All blunders → low accuracy.
	reviews := []MoveReview{
		{Color: "white", MoveNumber: 1, Classification: Blunder, ScoreBefore: 50, ScoreAfter: -300, ScoreDelta: -350},
		{Color: "white", MoveNumber: 2, Classification: Blunder, ScoreBefore: 50, ScoreAfter: -350, ScoreDelta: -400},
	}

	summary := Summarize(reviews, "", "", "", "")

	assert.Less(t, summary.White.Accuracy, 50.0)
}

func TestSummarize_PhaseAccuracy(t *testing.T) {
	t.Parallel()

	reviews := []MoveReview{
		// Opening moves (1–10): perfect play.
		{Color: "white", MoveNumber: 1, Classification: Best, ScoreBefore: 15, ScoreAfter: 15, ScoreDelta: 0},
		{Color: "white", MoveNumber: 5, Classification: Best, ScoreBefore: 15, ScoreAfter: 15, ScoreDelta: 0},
		// Middlegame moves (11–25): some loss.
		{Color: "white", MoveNumber: 15, Classification: Inaccuracy, ScoreBefore: 50, ScoreAfter: -10, ScoreDelta: -60},
		// No endgame moves for white.
	}

	summary := Summarize(reviews, "", "", "", "")

	// Opening accuracy should be near 100 (zero loss).
	assert.InDelta(t, 100.0, summary.White.PhaseAccuracy[Opening], 1.0)

	// Middlegame accuracy should be lower.
	assert.Less(t, summary.White.PhaseAccuracy[Middlegame], summary.White.PhaseAccuracy[Opening])

	// Endgame should be NaN (no moves).
	assert.True(t, math.IsNaN(summary.White.PhaseAccuracy[Endgame]))
}

func TestSummarize_SentinelExcludedFromAccuracy(t *testing.T) {
	t.Parallel()

	// A Miss move has a sentinel-sized delta and should be excluded from accuracy.
	// Without exclusion it would massively reduce accuracy.
	reviews := []MoveReview{
		{Color: "white", MoveNumber: 1, Classification: Best, ScoreBefore: 15, ScoreAfter: 15, ScoreDelta: 0},
		{Color: "white", MoveNumber: 2, Classification: Best, ScoreBefore: 15, ScoreAfter: 15, ScoreDelta: 0},
		{Color: "white", MoveNumber: 3, Classification: Miss, ScoreBefore: mateScoreSentinel, ScoreAfter: 0, ScoreDelta: -missThreshold},
	}

	summary := Summarize(reviews, "", "", "", "")

	// Accuracy should still be high since the sentinel is excluded.
	assert.Greater(t, summary.White.Accuracy, 90.0)
}

func TestSummarize_EmptyReviews(t *testing.T) {
	t.Parallel()

	summary := Summarize(nil, "X", "Y", "", "")

	assert.Equal(t, "X", summary.WhitePlayer)
	assert.Equal(t, "Y", summary.BlackPlayer)
	assert.True(t, math.IsNaN(summary.White.Accuracy))
	assert.True(t, math.IsNaN(summary.Black.Accuracy))
}

func TestSummarize_GameRatingInRange(t *testing.T) {
	t.Parallel()

	reviews := []MoveReview{
		{Color: "white", MoveNumber: 1, Classification: Best, ScoreBefore: 15, ScoreAfter: 15, ScoreDelta: 0},
		{Color: "black", MoveNumber: 1, Classification: Blunder, ScoreBefore: 50, ScoreAfter: -350, ScoreDelta: -400},
	}

	summary := Summarize(reviews, "", "", "", "")

	assert.GreaterOrEqual(t, summary.White.GameRating, gameRatingMin)
	assert.LessOrEqual(t, summary.White.GameRating, gameRatingMax)
	assert.GreaterOrEqual(t, summary.Black.GameRating, gameRatingMin)
	assert.LessOrEqual(t, summary.Black.GameRating, gameRatingMax)
}

func TestSummarize_BookMovesExcludedFromAccuracy(t *testing.T) {
	t.Parallel()

	// Book moves should not contribute to accuracy, regardless of their
	// score delta. Without exclusion the blunder-sized delta on the Book move
	// would collapse accuracy well below 90%.
	reviews := []MoveReview{
		{Color: "white", MoveNumber: 1, Classification: Book, ScoreBefore: 50, ScoreAfter: -350, ScoreDelta: -400},
		{Color: "white", MoveNumber: 2, Classification: Best, ScoreBefore: 15, ScoreAfter: 15, ScoreDelta: 0},
		{Color: "white", MoveNumber: 3, Classification: Best, ScoreBefore: 15, ScoreAfter: 15, ScoreDelta: 0},
	}

	summary := Summarize(reviews, "", "", "", "")

	// Only the two Best moves count; accuracy must be near 100.
	assert.Greater(t, summary.White.Accuracy, 90.0)
	// Book move must still be counted in ClassificationCounts.
	assert.Equal(t, 1, summary.White.ClassificationCounts[Book])
}

func TestSummarize_OpeningFields(t *testing.T) {
	t.Parallel()

	reviews := []MoveReview{
		{Color: "white", MoveNumber: 1, Classification: Book, ScoreBefore: 15, ScoreAfter: 15, ScoreDelta: 0},
	}

	summary := Summarize(reviews, "Alice", "Bob", "C50", "Italian Game")

	assert.Equal(t, "C50", summary.OpeningCode)
	assert.Equal(t, "Italian Game", summary.OpeningTitle)
}

func TestSummarize_OpeningFieldsEmpty(t *testing.T) {
	t.Parallel()

	reviews := []MoveReview{
		{Color: "white", MoveNumber: 1, Classification: Best, ScoreBefore: 15, ScoreAfter: 15, ScoreDelta: 0},
	}

	summary := Summarize(reviews, "", "", "", "")

	assert.Empty(t, summary.OpeningCode)
	assert.Empty(t, summary.OpeningTitle)
}

// -----------------------------------------------------------------------
// gameAccuracy (integration)
// -----------------------------------------------------------------------

func TestGameAccuracy_AllPerfect(t *testing.T) {
	t.Parallel()

	accs := []float64{100, 100, 100, 100, 100}
	wps := []float64{50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50}

	got := gameAccuracy(accs, wps)
	assert.InDelta(t, 100.0, got, 0.1)
}

func TestGameAccuracy_SingleMove(t *testing.T) {
	t.Parallel()

	accs := []float64{75.0}
	wps := []float64{50, 50, 45}

	got := gameAccuracy(accs, wps)
	assert.InDelta(t, 75.0, got, 0.1)
}
