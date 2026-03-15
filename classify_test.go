package chessreview

import (
	"math"
	"testing"

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
		isSacrifice bool
		isBook      bool
		expected    Classification
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
			playedMove:  "d2d4",
			bestMove:    "e2e4",
			isSacrifice: true,
			isBook:      true,
			expected:    Book,
		},
		// --- Best move cases ---
		// A large cp loss where the move is still the engine best (played == best)
		// and no special conditions trigger → Best.
		{
			name:        "best move returns Best",
			scoreBefore: 0, scoreAfter: -50,
			playedMove: "e2e4",
			bestMove:   "e2e4",
			expected:   Best,
		},
		// --- Excellent cases (0–2% win-prob loss) ---
		// From equal (0 cp), a 0 cp change → 0% loss → Excellent.
		{
			name:        "zero loss from equal returns Excellent",
			scoreBefore: 0, scoreAfter: 0,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Excellent,
		},
		// Positive delta (position improved beyond what best expected) → Excellent.
		{
			name:        "positive delta (improvement) returns Excellent",
			scoreBefore: 0, scoreAfter: 50,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Excellent,
		},
		// From 0 cp, a 15 cp loss: winProb(0)=0.50, winProb(-15)≈0.481 → loss≈1.9% < 2% → Excellent.
		{
			name:        "small cp loss from equal stays Excellent",
			scoreBefore: 0, scoreAfter: -15,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Excellent,
		},
		// --- Good cases (2–5% win-prob loss) ---
		// From 0 cp, a 60 cp loss: winProb(0)=0.50, winProb(-60)≈0.465 → loss≈3.5% → Good.
		{
			name:        "moderate cp loss from equal returns Good",
			scoreBefore: 0, scoreAfter: -60,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Good,
		},
		// --- Inaccuracy cases (5–10% win-prob loss) ---
		// From 0 cp, a 150 cp loss: winProb(0)=0.50, winProb(-150)≈0.407 → loss≈9.3% → Inaccuracy.
		{
			name:        "150 cp loss from equal returns Inaccuracy",
			scoreBefore: 0, scoreAfter: -150,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Inaccuracy,
		},
		// --- Mistake cases (10–20% win-prob loss) ---
		// From 0 cp, a 300 cp loss: winProb(0)=0.50, winProb(-300)≈0.325 → loss≈17.5% → Mistake.
		{
			name:        "300 cp loss from equal returns Mistake",
			scoreBefore: 0, scoreAfter: -300,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Mistake,
		},
		// --- Blunder cases (>20% win-prob loss) ---
		// From 0 cp, a 600 cp loss: winProb(0)=0.50, winProb(-600)≈0.269 → loss≈23.1% → Blunder.
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
		// Equal position, sacrifice, zero loss → Brilliant.
		{
			name:        "sacrifice with zero loss in equal position returns Brilliant",
			scoreBefore: 50, scoreAfter: 50,
			playedMove:  "d2d4",
			bestMove:    "e2e4",
			isSacrifice: true,
			expected:    Brilliant,
		},
		// scoreBefore=50, scoreAfter=35 → winProb(50)≈0.512, winProb(35)≈0.509 → loss≈0.4% < 2% → Brilliant.
		{
			name:        "sacrifice with tiny win-prob loss returns Brilliant",
			scoreBefore: 50, scoreAfter: 35,
			playedMove:  "d2d4",
			bestMove:    "e2e4",
			isSacrifice: true,
			expected:    Brilliant,
		},
		// Sacrifice that is also the engine best → Brilliant takes priority over Best.
		{
			name:        "sacrifice that is also engine best returns Brilliant not Best",
			scoreBefore: 100, scoreAfter: 100,
			playedMove:  "e2e4",
			bestMove:    "e2e4",
			isSacrifice: true,
			expected:    Brilliant,
		},
		// scoreBefore=50, scoreAfter=-60 → winProb(50)≈0.531, winProb(-60)≈0.463 → loss≈6.8% > 2% → falls to Inaccuracy.
		{
			name:        "sacrifice with >2% win-prob loss falls through to Inaccuracy (not Brilliant)",
			scoreBefore: 50, scoreAfter: -60,
			playedMove:  "d2d4",
			bestMove:    "e2e4",
			isSacrifice: true,
			expected:    Inaccuracy,
		},
		// Brilliant suppressed: scoreBefore=200 (at threshold) → not below threshold → suppressed.
		{
			name:        "sacrifice suppressed when scoreBefore equals brilliantWinningThreshold (200 cp)",
			scoreBefore: 200, scoreAfter: 200,
			playedMove:  "e2e4",
			bestMove:    "e2e4",
			isSacrifice: true,
			expected:    Best,
		},
		// Brilliant suppressed: scoreBefore=300 > 200 → not Brilliant.
		{
			name:        "sacrifice suppressed when position clearly winning (300 cp > 200 threshold)",
			scoreBefore: 300, scoreAfter: 295,
			playedMove:  "d2d4",
			bestMove:    "e2e4",
			isSacrifice: true,
			expected:    Excellent,
		},
		// Brilliant suppressed: scoreBefore=500.
		{
			name:        "sacrifice suppressed when position winning (500 cp)",
			scoreBefore: 500, scoreAfter: 500,
			playedMove:  "e2e4",
			bestMove:    "e2e4",
			isSacrifice: true,
			expected:    Best,
		},
		// Brilliant suppressed: scoreBefore=900.
		{
			name:        "sacrifice suppressed when position overwhelmingly winning (900 cp)",
			scoreBefore: 900, scoreAfter: 900,
			playedMove:  "e2e4",
			bestMove:    "e2e4",
			isSacrifice: true,
			expected:    Best,
		},
		// Non-sacrifice with tiny loss → Excellent, not Brilliant.
		{
			name:        "non-sacrifice with tiny loss returns Excellent not Brilliant",
			scoreBefore: 50, scoreAfter: 45,
			playedMove:  "d2d4",
			bestMove:    "e2e4",
			isSacrifice: false,
			expected:    Excellent,
		},
		// --- Great move cases ---
		// Rescue from losing (winProb < 0.40) into equal territory.
		// scoreBefore=-250 → winProb≈0.269 (<0.40); scoreAfter=0 → winProb=0.50 (≥0.40).
		// win-prob loss = winProb(-250)-winProb(0) < 0 → clamped to 0 → ≤2% → Great.
		{
			name:        "rescue from losing to equal returns Great",
			scoreBefore: -250, scoreAfter: 0,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Great,
		},
		// Convert equal to winning: scoreBefore=0 → winProb=0.50 (<0.60); scoreAfter=400 → winProb≈0.731 (≥0.60).
		// win-prob loss < 0 → clamped to 0 → ≤2% → Great.
		{
			name:        "equal to winning conversion returns Great",
			scoreBefore: 0, scoreAfter: 400,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Great,
		},
		// Sacrifice AND a turning-point: Brilliant takes priority when scoreBefore < 200 cp.
		{
			name:        "sacrifice in equal position takes Brilliant priority over Great",
			scoreBefore: 50, scoreAfter: 400,
			playedMove:  "d2d4",
			bestMove:    "e2e4",
			isSacrifice: true,
			expected:    Brilliant,
		},
		// Great: rescue from losing, and it is also the best move → Great takes priority over Best.
		{
			name:        "turning-point that is also best move returns Great not Best",
			scoreBefore: -250, scoreAfter: 0,
			playedMove: "d2d4",
			bestMove:   "d2d4",
			expected:   Great,
		},
		// Not Great: position was already ≥ greatWinningThreshold before the move.
		// scoreBefore=400 → winProb≈0.731 (≥0.60) → no swing from outside to inside → not Great.
		{
			name:        "already winning before move does not trigger Great",
			scoreBefore: 400, scoreAfter: 600,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Excellent,
		},
		// Not Great: losing position stays losing → no rescue → Excellent (position improved slightly).
		// scoreBefore=-300→winProb≈0.325 (<0.40), scoreAfter=-250→winProb≈0.269 (<0.40) → not rescued → not Great.
		{
			name:        "losing to still-losing move is not Great",
			scoreBefore: -300, scoreAfter: -250,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Excellent,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := Classify(tc.scoreBefore, tc.scoreAfter, tc.playedMove, tc.bestMove, tc.isSacrifice, tc.isBook)

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
		{name: "+400 cp is ~0.731", cp: 400, wantMin: 0.72, wantMax: 0.74},
		{name: "-400 cp is ~0.269", cp: -400, wantMin: 0.26, wantMax: 0.28},
		{name: "+inf tends to 1.0", cp: 10000, wantMin: 0.999, wantMax: 1.0},
		{name: "-inf tends to 0.0", cp: -10000, wantMin: 0.0, wantMax: 0.001},
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
			name:        "loss from equal to -300 cp is ~17.5%",
			scoreBefore: 0, scoreAfter: -300,
			wantMin: 0.17, wantMax: 0.18,
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
