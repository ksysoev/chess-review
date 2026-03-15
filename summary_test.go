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
// cpLoss
// -----------------------------------------------------------------------

func TestCpLoss(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		scoreDelta int
		want       int
	}{
		{name: "positive delta (improvement) returns 0", scoreDelta: 50, want: 0},
		{name: "zero delta returns 0", scoreDelta: 0, want: 0},
		{name: "small loss", scoreDelta: -30, want: 30},
		{name: "large loss below sentinel", scoreDelta: -(missThreshold - 1), want: missThreshold - 1},
		{name: "loss at sentinel excluded", scoreDelta: -missThreshold, want: -1},
		{name: "loss above sentinel excluded", scoreDelta: -(missThreshold + 100), want: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, cpLoss(tt.scoreDelta))
		})
	}
}

// -----------------------------------------------------------------------
// calcAccuracy
// -----------------------------------------------------------------------

func TestCalcAccuracy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		avgCPL  float64
		wantMin float64
		wantMax float64
	}{
		{name: "perfect play (0 cpl) near 100", avgCPL: 0, wantMin: 99.0, wantMax: 100.0},
		{name: "very high loss clamped to 0", avgCPL: 1000, wantMin: 0, wantMax: 0},
		{name: "moderate loss mid-range", avgCPL: 50, wantMin: 5, wantMax: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := calcAccuracy(tt.avgCPL)
			assert.GreaterOrEqual(t, got, tt.wantMin)
			assert.LessOrEqual(t, got, tt.wantMax)
		})
	}
}

func TestCalcAccuracy_NeverExceedsBounds(t *testing.T) {
	t.Parallel()

	for cpl := 0.0; cpl <= 500; cpl += 5 {
		got := calcAccuracy(cpl)
		assert.GreaterOrEqual(t, got, 0.0, "accuracy below 0 at avgCPL=%.0f", cpl)
		assert.LessOrEqual(t, got, 100.0, "accuracy above 100 at avgCPL=%.0f", cpl)
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
		{name: "0% accuracy returns min", accuracy: 0, wantMin: gameRatingMin, wantMax: gameRatingMin},
		{name: "100% accuracy returns expected rating", accuracy: 100, wantMin: 2700, wantMax: 2700},
		{name: "50% accuracy mid-range", accuracy: 50, wantMin: gameRatingMin, wantMax: gameRatingMax},
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
		{Color: "white", MoveNumber: 1, Classification: Best, ScoreDelta: 0},
		{Color: "black", MoveNumber: 1, Classification: Good, ScoreDelta: -15},
	}

	summary := Summarize(reviews, "Alice", "Bob", "", "")

	assert.Equal(t, "Alice", summary.WhitePlayer)
	assert.Equal(t, "Bob", summary.BlackPlayer)
}

func TestSummarize_ClassificationCounts(t *testing.T) {
	t.Parallel()

	reviews := []MoveReview{
		{Color: "white", MoveNumber: 1, Classification: Best, ScoreDelta: 0},
		{Color: "white", MoveNumber: 2, Classification: Brilliant, ScoreDelta: -5},
		{Color: "white", MoveNumber: 3, Classification: Inaccuracy, ScoreDelta: -50},
		{Color: "black", MoveNumber: 1, Classification: Blunder, ScoreDelta: -400},
		{Color: "black", MoveNumber: 2, Classification: Blunder, ScoreDelta: -350},
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
		{Color: "white", MoveNumber: 1, Classification: Best, ScoreDelta: 0},
		{Color: "white", MoveNumber: 2, Classification: Best, ScoreDelta: 0},
		{Color: "white", MoveNumber: 3, Classification: Best, ScoreDelta: 0},
	}

	summary := Summarize(reviews, "", "", "", "")

	assert.InDelta(t, 100.0, summary.White.Accuracy, 1.0)
}

func TestSummarize_AccuracyBadPlay(t *testing.T) {
	t.Parallel()

	// All blunders → low accuracy.
	reviews := []MoveReview{
		{Color: "white", MoveNumber: 1, Classification: Blunder, ScoreDelta: -350},
		{Color: "white", MoveNumber: 2, Classification: Blunder, ScoreDelta: -400},
	}

	summary := Summarize(reviews, "", "", "", "")

	assert.Less(t, summary.White.Accuracy, 50.0)
}

func TestSummarize_PhaseAccuracy(t *testing.T) {
	t.Parallel()

	reviews := []MoveReview{
		// Opening moves (1–10)
		{Color: "white", MoveNumber: 1, Classification: Best, ScoreDelta: 0},
		{Color: "white", MoveNumber: 5, Classification: Best, ScoreDelta: 0},
		// Middlegame moves (11–25)
		{Color: "white", MoveNumber: 15, Classification: Inaccuracy, ScoreDelta: -60},
		// No endgame moves for white
	}

	summary := Summarize(reviews, "", "", "", "")

	// Opening accuracy should be near 100 (zero CPL).
	assert.InDelta(t, 100.0, summary.White.PhaseAccuracy[Opening], 1.0)

	// Middlegame accuracy should be lower.
	assert.Less(t, summary.White.PhaseAccuracy[Middlegame], summary.White.PhaseAccuracy[Opening])

	// Endgame should be NaN (no moves).
	assert.True(t, math.IsNaN(summary.White.PhaseAccuracy[Endgame]))
}

func TestSummarize_SentinelExcludedFromCPL(t *testing.T) {
	t.Parallel()

	// A Miss move has a sentinel-sized delta and should be excluded from CPL.
	// Without exclusion it would massively reduce accuracy.
	reviews := []MoveReview{
		{Color: "white", MoveNumber: 1, Classification: Best, ScoreDelta: 0},
		{Color: "white", MoveNumber: 2, Classification: Best, ScoreDelta: 0},
		{Color: "white", MoveNumber: 3, Classification: Miss, ScoreDelta: -missThreshold},
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
		{Color: "white", MoveNumber: 1, Classification: Best, ScoreDelta: 0},
		{Color: "black", MoveNumber: 1, Classification: Blunder, ScoreDelta: -400},
	}

	summary := Summarize(reviews, "", "", "", "")

	assert.GreaterOrEqual(t, summary.White.GameRating, gameRatingMin)
	assert.LessOrEqual(t, summary.White.GameRating, gameRatingMax)
	assert.GreaterOrEqual(t, summary.Black.GameRating, gameRatingMin)
	assert.LessOrEqual(t, summary.Black.GameRating, gameRatingMax)
}

func TestSummarize_BookMovesExcludedFromCPL(t *testing.T) {
	t.Parallel()

	// Book moves should not contribute to CPL / accuracy, regardless of their
	// score delta. Without exclusion the blunder-sized delta on the Book move
	// would collapse accuracy well below 90%.
	reviews := []MoveReview{
		{Color: "white", MoveNumber: 1, Classification: Book, ScoreDelta: -400},
		{Color: "white", MoveNumber: 2, Classification: Best, ScoreDelta: 0},
		{Color: "white", MoveNumber: 3, Classification: Best, ScoreDelta: 0},
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
		{Color: "white", MoveNumber: 1, Classification: Book, ScoreDelta: 0},
	}

	summary := Summarize(reviews, "Alice", "Bob", "C50", "Italian Game")

	assert.Equal(t, "C50", summary.OpeningCode)
	assert.Equal(t, "Italian Game", summary.OpeningTitle)
}

func TestSummarize_OpeningFieldsEmpty(t *testing.T) {
	t.Parallel()

	reviews := []MoveReview{
		{Color: "white", MoveNumber: 1, Classification: Best, ScoreDelta: 0},
	}

	summary := Summarize(reviews, "", "", "", "")

	assert.Empty(t, summary.OpeningCode)
	assert.Empty(t, summary.OpeningTitle)
}
