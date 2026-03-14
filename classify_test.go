package chessreview

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		playedMove string
		bestMove   string
		scoreDelta int
		expected   Classification
	}{
		{
			name:       "best move returns Best",
			scoreDelta: -50,
			playedMove: "e2e4",
			bestMove:   "e2e4",
			expected:   Best,
		},
		{
			name:       "zero loss returns Excellent",
			scoreDelta: 0,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Excellent,
		},
		{
			name:       "10 cp loss returns Excellent",
			scoreDelta: -10,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Excellent,
		},
		{
			name:       "11 cp loss returns Good",
			scoreDelta: -11,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Good,
		},
		{
			name:       "25 cp loss returns Good",
			scoreDelta: -25,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Good,
		},
		{
			name:       "26 cp loss returns Inaccuracy",
			scoreDelta: -26,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Inaccuracy,
		},
		{
			name:       "100 cp loss returns Inaccuracy",
			scoreDelta: -100,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Inaccuracy,
		},
		{
			name:       "101 cp loss returns Mistake",
			scoreDelta: -101,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Mistake,
		},
		{
			name:       "300 cp loss returns Mistake",
			scoreDelta: -300,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Mistake,
		},
		{
			name:       "301 cp loss returns Blunder",
			scoreDelta: -301,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Blunder,
		},
		{
			name:       "large loss returns Blunder",
			scoreDelta: -800,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Blunder,
		},
		{
			name:       "positive delta (improvement) with different move returns Excellent",
			scoreDelta: 50,
			playedMove: "d2d4",
			bestMove:   "e2e4",
			expected:   Excellent,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := Classify(tc.scoreDelta, tc.playedMove, tc.bestMove)

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
		{expected: "Best", classification: Best},
		{expected: "Excellent", classification: Excellent},
		{expected: "Good", classification: Good},
		{expected: "Inaccuracy", classification: Inaccuracy},
		{expected: "Mistake", classification: Mistake},
		{expected: "Blunder", classification: Blunder},
		{expected: "Miss", classification: Miss},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, tc.classification.String())
		})
	}
}
