package chessreview

import (
	"math"
	"testing"

	"github.com/corentings/chess/v2"
	"github.com/stretchr/testify/assert"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		playedMove  string
		bestMove    string
		scoreBefore int
		scoreAfter  int
		// 2-ply lookback fields
		scoreBeforePrev     int
		hasPrev             bool
		isSacrifice         bool
		sacrificedPieceType chess.PieceType
		isBook              bool
		expected            Classification
	}{
		// --- Book move cases ---
		{
			name:        "book move returns Book regardless of delta",
			scoreBefore: 0, scoreAfter: -50,
			playedMove: "e2e4",
			bestMove:   "e2e4",
			isBook:     true,
			expected:   Book,
		},
		{
			name:        "book move returns Book even when not best",
			scoreBefore: 0, scoreAfter: -200,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			isBook:     true,
			expected:   Book,
		},
		{
			name:        "book move returns Book even when sacrifice",
			scoreBefore: 50, scoreAfter: 50,
			playedMove:          "d2d4",
			bestMove:            "e2e4",
			isSacrifice:         true,
			sacrificedPieceType: chess.Knight,
			isBook:              true,
			expected:            Book,
		},
		// --- Best move cases ---
		{
			name:        "best move returns Best",
			scoreBefore: 0, scoreAfter: -50,
			playedMove: "e2e4",
			bestMove:   "e2e4",
			expected:   Best,
		},
		// --- Excellent cases (0–2% win-prob loss) ---
		{
			name:        "zero loss from equal returns Excellent",
			scoreBefore: 0, scoreAfter: 0,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Excellent,
		},
		{
			name:        "positive delta (improvement) returns Excellent",
			scoreBefore: 0, scoreAfter: 50,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Excellent,
		},
		// From 0 cp, a 15 cp loss: with Lichess coeff the loss is ~1.4% < 2% → Excellent.
		{
			name:        "small cp loss from equal stays Excellent",
			scoreBefore: 0, scoreAfter: -15,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Excellent,
		},
		// --- Good cases (2–5% win-prob loss) ---
		// With the Lichess coefficient, a 30 cp loss from equal gives ~2.7% → Good.
		{
			name:        "30 cp loss from equal returns Good",
			scoreBefore: 0, scoreAfter: -30,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Good,
		},
		// --- Inaccuracy cases (5–10% win-prob loss) ---
		// With the Lichess coefficient, a 60 cp loss from equal gives ~5.5% → Inaccuracy.
		{
			name:        "60 cp loss from equal returns Inaccuracy",
			scoreBefore: 0, scoreAfter: -60,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Inaccuracy,
		},
		// --- Mistake cases (10–20% win-prob loss) ---
		// With the Lichess coefficient, a 150 cp loss from equal gives ~13.5% → Mistake.
		{
			name:        "150 cp loss from equal returns Mistake",
			scoreBefore: 0, scoreAfter: -150,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Mistake,
		},
		// --- Blunder cases (>20% win-prob loss) ---
		// With the Lichess coefficient, a 300 cp loss from equal gives ~25.1% → Blunder.
		{
			name:        "300 cp loss from equal returns Blunder",
			scoreBefore: 0, scoreAfter: -300,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Blunder,
		},
		{
			name:        "600 cp loss from equal returns Blunder",
			scoreBefore: 0, scoreAfter: -600,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Blunder,
		},
		{
			name:        "large loss returns Blunder",
			scoreBefore: 0, scoreAfter: -800,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Blunder,
		},
		// --- Brilliant move cases ---
		{
			name:        "sacrifice that is the best move with maintained score returns Brilliant",
			scoreBefore: 50, scoreAfter: 50,
			playedMove:          "e2e4",
			bestMove:            "e2e4",
			isSacrifice:         true,
			sacrificedPieceType: chess.Knight,
			expected:            Brilliant,
		},
		{
			name:        "sacrifice that is the best move with improved score returns Brilliant",
			scoreBefore: 50, scoreAfter: 80,
			playedMove:          "e2e4",
			bestMove:            "e2e4",
			isSacrifice:         true,
			sacrificedPieceType: chess.Knight,
			expected:            Brilliant,
		},
		{
			name:        "sacrifice that is engine best but worsens position returns Best not Brilliant",
			scoreBefore: 50, scoreAfter: 35,
			playedMove:          "e2e4",
			bestMove:            "e2e4",
			isSacrifice:         true,
			sacrificedPieceType: chess.Knight,
			expected:            Best,
		},
		{
			name:        "sacrifice not the best move returns Excellent not Brilliant",
			scoreBefore: 50, scoreAfter: 50,
			playedMove:          "d2d4",
			bestMove:            "e2e4",
			isSacrifice:         true,
			sacrificedPieceType: chess.Knight,
			expected:            Excellent,
		},
		{
			name:        "sacrifice that is also engine best returns Brilliant not Best",
			scoreBefore: 100, scoreAfter: 100,
			playedMove:          "e2e4",
			bestMove:            "e2e4",
			isSacrifice:         true,
			sacrificedPieceType: chess.Bishop,
			expected:            Brilliant,
		},
		// Sacrifice with large loss, not best → Mistake. Using the Lichess coefficient:
		// winProb(50) ≈ 0.546, winProb(-60) ≈ 0.445, loss ≈ 0.101 (~10.1%) → Mistake.
		{
			name:        "sacrifice with large win-prob loss falls through to Mistake (not Brilliant)",
			scoreBefore: 50, scoreAfter: -60,
			playedMove:          "d2d4",
			bestMove:            "e2e4",
			isSacrifice:         true,
			sacrificedPieceType: chess.Knight,
			expected:            Mistake,
		},
		// Brilliant suppressed: scoreBefore=200 (at threshold).
		{
			name:        "sacrifice suppressed when scoreBefore equals brilliantWinningThreshold (200 cp)",
			scoreBefore: 200, scoreAfter: 200,
			playedMove:          "e2e4",
			bestMove:            "e2e4",
			isSacrifice:         true,
			sacrificedPieceType: chess.Knight,
			expected:            Best,
		},
		// Brilliant suppressed: scoreBefore=300 > 200.
		{
			name:        "sacrifice suppressed when position clearly winning (300 cp > 200 threshold)",
			scoreBefore: 300, scoreAfter: 295,
			playedMove:          "d2d4",
			bestMove:            "e2e4",
			isSacrifice:         true,
			sacrificedPieceType: chess.Knight,
			expected:            Excellent,
		},
		{
			name:        "sacrifice suppressed when position winning (500 cp)",
			scoreBefore: 500, scoreAfter: 500,
			playedMove:          "e2e4",
			bestMove:            "e2e4",
			isSacrifice:         true,
			sacrificedPieceType: chess.Rook,
			expected:            Best,
		},
		{
			name:        "sacrifice suppressed when position overwhelmingly winning (900 cp)",
			scoreBefore: 900, scoreAfter: 900,
			playedMove:          "e2e4",
			bestMove:            "e2e4",
			isSacrifice:         true,
			sacrificedPieceType: chess.Queen,
			expected:            Best,
		},
		{
			name:        "non-sacrifice with tiny loss returns Excellent not Brilliant",
			scoreBefore: 50, scoreAfter: 45,
			playedMove:  "d2d4",
			bestMove:    "e2e4",
			isSacrifice: false,
			expected:    Excellent,
		},
		{
			name:        "pawn sacrifice is excluded from Brilliant even when all other conditions met",
			scoreBefore: 50, scoreAfter: 55,
			playedMove:          "b2b4",
			bestMove:            "b2b4",
			isSacrifice:         true,
			sacrificedPieceType: chess.Pawn,
			expected:            Best,
		},
		{
			name:        "sacrifice with NoPieceType is excluded from Brilliant (fail-closed guard)",
			scoreBefore: 50, scoreAfter: 55,
			playedMove:          "e2e4",
			bestMove:            "e2e4",
			isSacrifice:         true,
			sacrificedPieceType: chess.NoPieceType,
			expected:            Best,
		},
		// --- Great move cases (1-ply) ---
		// Rescue from losing (winProb < 0.40) into equal territory.
		// With Lichess coeff: winProb(-250) ≈ 0.286 (<0.40); winProb(0) = 0.50 (≥0.40).
		{
			name:        "rescue from losing to equal returns Great",
			scoreBefore: -250, scoreAfter: 0,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Great,
		},
		// Convert equal to winning.
		// With Lichess coeff: winProb(0) = 0.50 (<0.60); winProb(400) ≈ 0.814 (≥0.60).
		{
			name:        "equal to winning conversion returns Great",
			scoreBefore: 0, scoreAfter: 400,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Great,
		},
		{
			name:        "sacrifice turning-point without being best move returns Great not Brilliant",
			scoreBefore: 50, scoreAfter: 400,
			playedMove:          "d2d4",
			bestMove:            "e2e4",
			isSacrifice:         true,
			sacrificedPieceType: chess.Knight,
			expected:            Great,
		},
		{
			name:        "sacrifice turning-point that is also best move returns Brilliant over Great",
			scoreBefore: 50, scoreAfter: 400,
			playedMove:          "e2e4",
			bestMove:            "e2e4",
			isSacrifice:         true,
			sacrificedPieceType: chess.Knight,
			expected:            Brilliant,
		},
		{
			name:        "turning-point that is also best move returns Great not Best",
			scoreBefore: -250, scoreAfter: 0,
			playedMove: "d2d4",
			bestMove:   "d2d4",
			expected:   Great,
		},
		// Not Great: position was already ≥ greatWinningThreshold before the move.
		// With Lichess coeff: winProb(400) ≈ 0.814 (≥0.60).
		{
			name:        "already winning before move does not trigger Great",
			scoreBefore: 400, scoreAfter: 600,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Excellent,
		},
		// Not Great: losing → still losing.
		// With Lichess coeff: winProb(-300) ≈ 0.249 (<0.40), winProb(-250) ≈ 0.286 (<0.40).
		{
			name:        "losing to still-losing move is not Great",
			scoreBefore: -300, scoreAfter: -250,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Excellent,
		},
		// --- Great move cases (2-ply lookback) ---
		// With Lichess coeff: winProb(108) ≈ 0.598 (<0.60); winProb(411) ≈ 0.820 (≥0.60).
		{
			name:        "capitalise on opponent blunder via 2-ply lookback returns Great",
			scoreBefore: 411, scoreAfter: 411,
			scoreBeforePrev: 108, hasPrev: true,
			playedMove: "c5e6",
			bestMove:   "c5e6",
			expected:   Great,
		},
		// With Lichess coeff: winProb(-250) ≈ 0.286 (<0.40); winProb(50) ≈ 0.519 (≥0.40).
		{
			name:        "2-ply lookback rescue from losing via opponent blunder returns Great",
			scoreBefore: 50, scoreAfter: 50,
			scoreBeforePrev: -250, hasPrev: true,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Great,
		},
		{
			name:        "2-ply lookback suppressed when hasPrev is false",
			scoreBefore: 411, scoreAfter: 411,
			scoreBeforePrev: 108, hasPrev: false,
			playedMove: "c5e6",
			bestMove:   "c5e6",
			expected:   Best,
		},
		// With Lichess coeff: winProb(400) ≈ 0.814 (≥0.60) → already winning.
		{
			name:        "2-ply lookback does not fire when player was already winning two moves ago",
			scoreBefore: 500, scoreAfter: 500,
			scoreBeforePrev: 400, hasPrev: true,
			playedMove: "e2e4",
			bestMove:   "e2e4",
			expected:   Best,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := ClassifyContext{
				PlayedMove:          tc.playedMove,
				BestMove:            tc.bestMove,
				ScoreBefore:         tc.scoreBefore,
				ScoreAfter:          tc.scoreAfter,
				ScoreBeforePrev:     tc.scoreBeforePrev,
				HasPrev:             tc.hasPrev,
				IsSacrifice:         tc.isSacrifice,
				SacrificedPieceType: tc.sacrificedPieceType,
				IsBook:              tc.isBook,
			}
			result := Classify(ctx)

			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestClassificationString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected       string
		classification Classification
	}{
		{expected: "Book", classification: Book},
		{expected: "Brilliant", classification: Brilliant},
		{expected: "Great", classification: Great},
		{expected: "Best", classification: Best},
		{expected: "Excellent", classification: Excellent},
		{expected: "Good", classification: Good},
		{expected: "Inaccuracy", classification: Inaccuracy},
		{expected: "Mistake", classification: Mistake},
		{expected: "Blunder", classification: Blunder},
		{expected: "Miss", classification: Miss},
		{expected: "Unknown", classification: Classification(99)},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, tc.classification.String())
		})
	}
}

func TestWinProb(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cp      int
		wantMin float64
		wantMax float64
	}{
		{name: "equal position is 0.50", cp: 0, wantMin: 0.50, wantMax: 0.50},
		// With Lichess coeff: winProb(400) ≈ 0.814.
		{name: "+400 cp is ~0.81", cp: 400, wantMin: 0.80, wantMax: 0.83},
		{name: "-400 cp is ~0.19", cp: -400, wantMin: 0.17, wantMax: 0.20},
		// Capped at ±1000: winProb(1000) ≈ 0.976.
		{name: "+10000 capped to +1000 near 0.976", cp: 10000, wantMin: 0.97, wantMax: 0.98},
		{name: "-10000 capped to -1000 near 0.024", cp: -10000, wantMin: 0.02, wantMax: 0.03},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := winProb(tt.cp)
			assert.GreaterOrEqual(t, got, tt.wantMin)
			assert.LessOrEqual(t, got, tt.wantMax)
		})
	}
}

func TestWinPercent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cp      int
		wantMin float64
		wantMax float64
	}{
		{name: "equal position is 50.0", cp: 0, wantMin: 50.0, wantMax: 50.0},
		{name: "+400 cp is ~81.4", cp: 400, wantMin: 80.0, wantMax: 83.0},
		{name: "-400 cp is ~18.6", cp: -400, wantMin: 17.0, wantMax: 20.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := winPercent(tt.cp)
			assert.GreaterOrEqual(t, got, tt.wantMin)
			assert.LessOrEqual(t, got, tt.wantMax)
		})
	}
}

func TestWinProbLoss(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		scoreBefore int
		scoreAfter  int
		wantMin     float64
		wantMax     float64
	}{
		{
			name:        "no change → zero loss",
			scoreBefore: 0, scoreAfter: 0,
			wantMin: 0.0, wantMax: 0.0,
		},
		{
			name:        "improvement → zero loss (clamped)",
			scoreBefore: 0, scoreAfter: 100,
			wantMin: 0.0, wantMax: 0.0,
		},
		{
			// With Lichess coeff: winProb(0)=0.50, winProb(-300)≈0.249 → loss≈0.251.
			name:        "loss from equal to -300 cp is ~25.1%",
			scoreBefore: 0, scoreAfter: -300,
			wantMin: 0.24, wantMax: 0.26,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := winProbLoss(tt.scoreBefore, tt.scoreAfter)
			assert.GreaterOrEqual(t, got, tt.wantMin)
			assert.LessOrEqual(t, got, tt.wantMax)
		})
	}
}

// TestWinProbSymmetry verifies that winProb(cp) + winProb(-cp) == 1 (symmetry
// of the sigmoid around 0).
func TestWinProbSymmetry(t *testing.T) {
	t.Parallel()

	for _, cp := range []int{0, 50, 100, 200, 400, 800} {
		assert.InDelta(t, 1.0, winProb(cp)+winProb(-cp), 1e-9,
			"symmetry broken at cp=%d", cp)
	}
}

// TestWinProbMonotonic verifies that a higher centipawn score always gives a
// higher win probability.
func TestWinProbMonotonic(t *testing.T) {
	t.Parallel()

	prev := math.Inf(-1)

	for cp := -1000; cp <= 1000; cp += 50 {
		got := winProb(cp)
		assert.Greater(t, got, prev, "winProb not monotonically increasing at cp=%d", cp)
		prev = got
	}
}

// TestWinProbClamped verifies that values beyond ±1000 are capped.
func TestWinProbClamped(t *testing.T) {
	t.Parallel()

	assert.InDelta(t, winProb(1000), winProb(5000), 1e-12,
		"winProb should clamp values > 1000")
	assert.InDelta(t, winProb(-1000), winProb(-5000), 1e-12,
		"winProb should clamp values < -1000")
}
