package chessreview

import (
	"testing"

	"github.com/notnil/chess"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTestPositions parses fen, finds the legal move matching uciMove, applies it,
// and returns (beforePos, afterPos, move). It fails the test immediately if the
// FEN is invalid or the move is not found in the legal move list.
func getTestPositions(t *testing.T, fen, uciMove string) (beforePos, afterPos *chess.Position, move *chess.Move) {
	t.Helper()

	fenOpt, err := chess.FEN(fen)
	require.NoError(t, err, "invalid FEN")

	game := chess.NewGame(fenOpt)
	beforePos = game.Position()

	for _, m := range beforePos.ValidMoves() {
		if moveToUCI(m) == uciMove {
			move = m
			break
		}
	}

	require.NotNil(t, move, "move %q not found in legal moves for FEN %q", uciMove, fen)

	require.NoError(t, game.Move(move))

	afterPos = game.Position()

	return beforePos, afterPos, move
}

func TestPieceTypeValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		piece    chess.PieceType
		expected int
	}{
		{name: "Pawn", piece: chess.Pawn, expected: pawnValue},
		{name: "Knight", piece: chess.Knight, expected: knightValue},
		{name: "Bishop", piece: chess.Bishop, expected: bishopValue},
		{name: "Rook", piece: chess.Rook, expected: rookValue},
		{name: "Queen", piece: chess.Queen, expected: queenValue},
		{name: "King", piece: chess.King, expected: kingValue},
		{name: "NoPieceType", piece: chess.NoPieceType, expected: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, pieceTypeValue(tc.piece))
		})
	}
}

func TestDetectSacrifice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fen      string
		uciMove  string
		expected bool
	}{
		{
			// White Queen (900) captures a Black pawn (100) on f5.
			// The Black pawn on g6 can recapture on f5 → net: Queen for pawn → sacrifice.
			name:     "queen captures pawn on pawn-defended square",
			fen:      "7k/8/6p1/5p2/4Q3/8/8/4K3 w - - 0 1",
			uciMove:  "e4f5",
			expected: true,
		},
		{
			// White Rook (500) captures a Black Rook (500) on f6.
			// capturedValue == movedValue → even exchange, not a sacrifice.
			// Rook on f5 moves one square vertically to f6 (valid rook move).
			name:     "rook captures rook — equal exchange is not a sacrifice",
			fen:      "7k/6p1/5r2/5R2/8/8/8/4K3 w - - 0 1",
			uciMove:  "f5f6",
			expected: false,
		},
		{
			// White Knight (300) captures an isolated Black pawn (100) on f5.
			// The Black King on h8 cannot reach f5 in one move, so no recapture.
			name:     "knight captures undefended pawn — no recapture available",
			fen:      "7k/8/8/5p2/3N4/8/8/4K3 w - - 0 1",
			uciMove:  "d4f5",
			expected: false,
		},
		{
			// White Knight (300) moves to d5 (empty square).
			// The Black pawn on c6 can capture on d5 → White is giving away a Knight → sacrifice.
			name:     "knight moves to pawn-defended empty square",
			fen:      "7k/8/2p5/8/5N2/8/8/4K3 w - - 0 1",
			uciMove:  "f4d5",
			expected: true,
		},
		{
			// White Knight (300) captures the Black Queen (900) on d5.
			// capturedValue (900) > movedValue (300) → gaining material, not a sacrifice.
			name:     "knight captures queen — gaining material is not a sacrifice",
			fen:      "7k/8/8/3q4/5N2/8/8/4K3 w - - 0 1",
			uciMove:  "f4d5",
			expected: false,
		},
		{
			// White Rook (500) moves to e6 (empty square).
			// The Black pawn on d7 can capture on e6 → Rook for nothing → sacrifice.
			name:     "rook moves to pawn-defended empty square",
			fen:      "7k/3p4/8/8/8/4R3/8/4K3 w - - 0 1",
			uciMove:  "e3e6",
			expected: true,
		},
		{
			// En passant: White pawn (100) captures Black pawn (100) via en passant.
			// capturedValue == movedValue → even pawn trade, not a sacrifice.
			name:     "en passant pawn capture — equal exchange is not a sacrifice",
			fen:      "8/8/8/4Pp2/8/8/8/4K2k w - f6 0 1",
			uciMove:  "e5f6",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			before, after, move := getTestPositions(t, tc.fen, tc.uciMove)
			result := detectSacrifice(before, after, move)

			assert.Equal(t, tc.expected, result)
		})
	}
}
