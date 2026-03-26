package chessreview

import (
	"testing"

	"github.com/corentings/chess/v2"
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
		if moveToUCI(&m) == uciMove {
			move = &m
			break
		}
	}

	require.NotNil(t, move, "move %q not found in legal moves for FEN %q", uciMove, fen)

	require.NoError(t, game.Move(move, nil))

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
		name              string
		fen               string
		uciMove           string
		expected          bool
		expectedPieceType chess.PieceType
	}{
		{
			// White Queen (900) captures a Black pawn (100) on f5.
			// The Black pawn on g6 can recapture on f5 → net: Queen for pawn → sacrifice.
			name:              "queen captures pawn on pawn-defended square",
			fen:               "7k/8/6p1/5p2/4Q3/8/8/4K3 w - - 0 1",
			uciMove:           "e4f5",
			expected:          true,
			expectedPieceType: chess.Queen,
		},
		{
			// White Rook (500) captures a Black Rook (500) on f6.
			// capturedValue == movedValue → even exchange, not a sacrifice.
			// Rook on f5 moves one square vertically to f6 (valid rook move).
			name:              "rook captures rook — equal exchange is not a sacrifice",
			fen:               "7k/6p1/5r2/5R2/8/8/8/4K3 w - - 0 1",
			uciMove:           "f5f6",
			expected:          false,
			expectedPieceType: chess.NoPieceType,
		},
		{
			// White Knight (300) captures an isolated Black pawn (100) on f5.
			// The Black King on h8 cannot reach f5 in one move, so no recapture.
			name:              "knight captures undefended pawn — no recapture available",
			fen:               "7k/8/8/5p2/3N4/8/8/4K3 w - - 0 1",
			uciMove:           "d4f5",
			expected:          false,
			expectedPieceType: chess.NoPieceType,
		},
		{
			// White Knight (300) moves to d5 (empty square).
			// The Black pawn on c6 can capture on d5 → White is giving away a Knight → sacrifice.
			name:              "knight moves to pawn-defended empty square",
			fen:               "7k/8/2p5/8/5N2/8/8/4K3 w - - 0 1",
			uciMove:           "f4d5",
			expected:          true,
			expectedPieceType: chess.Knight,
		},
		{
			// White Knight (300) captures the Black Queen (900) on d5.
			// capturedValue (900) > movedValue (300) → gaining material, not a sacrifice.
			name:              "knight captures queen — gaining material is not a sacrifice",
			fen:               "7k/8/8/3q4/5N2/8/8/4K3 w - - 0 1",
			uciMove:           "f4d5",
			expected:          false,
			expectedPieceType: chess.NoPieceType,
		},
		{
			// White Rook (500) moves to e6 (empty square).
			// The Black pawn on d7 can capture on e6 → Rook for nothing → sacrifice.
			name:              "rook moves to pawn-defended empty square",
			fen:               "7k/3p4/8/8/8/4R3/8/4K3 w - - 0 1",
			uciMove:           "e3e6",
			expected:          true,
			expectedPieceType: chess.Rook,
		},
		{
			// En passant: White pawn (100) captures Black pawn (100) via en passant.
			// capturedValue == movedValue → even pawn trade, not a sacrifice.
			name:              "en passant pawn capture — equal exchange is not a sacrifice",
			fen:               "8/8/8/4Pp2/8/8/8/4K2k w - f6 0 1",
			uciMove:           "e5f6",
			expected:          false,
			expectedPieceType: chess.NoPieceType,
		},
		// --- Promotion cases ---
		{
			// White pawn on e7 captures the Black rook on d8 and promotes to queen.
			// movedValue = queenValue (900) > capturedValue = rookValue (500).
			// Black rook on c8 can recapture on d8 → sacrifice.
			// Verifies that move.Promo() is used instead of the pawn's value:
			// without the promotion branch movedValue would be pawnValue (100) < rookValue (500)
			// and the function would incorrectly return false.
			name:              "promotion to queen by capturing rook on rook-defended square",
			fen:               "2rr4/4P3/8/8/8/6K1/8/7k w - - 0 1",
			uciMove:           "e7d8q",
			expected:          true,
			expectedPieceType: chess.Queen,
		},
		{
			// White pawn on e7 promotes to queen on an empty, undefended square.
			// movedValue = queenValue (900) > capturedValue = 0, but no Black piece
			// can recapture on e8 → not a sacrifice.
			name:              "promotion to queen on empty undefended square",
			fen:               "8/4P3/8/8/8/8/8/4K2k w - - 0 1",
			uciMove:           "e7e8q",
			expected:          false,
			expectedPieceType: chess.NoPieceType,
		},
		{
			// White pawn on e7 captures the Black rook on d8 and under-promotes to knight.
			// movedValue = knightValue (300) < capturedValue = rookValue (500) →
			// material is gained, not sacrificed.
			// Verifies that the promoted type (knight) rather than pawn is used for comparison.
			name:              "under-promotion to knight capturing rook — gaining material is not a sacrifice",
			fen:               "3r4/4P3/8/8/8/6K1/8/7k w - - 0 1",
			uciMove:           "e7d8n",
			expected:          false,
			expectedPieceType: chess.NoPieceType,
		},
		{
			// White pawn on e7 promotes to queen on the empty e8 square.
			// Black rook on e1 can recapture on e8 in one move →
			// movedValue = queenValue (900) > capturedValue = 0 and recapture exists → sacrifice.
			// Verifies the promotion-without-capture branch correctly uses promoted piece value.
			// (White king is on g3 to avoid being in check from the rook on e1.)
			name:              "promotion to queen on empty square defended by rook",
			fen:               "8/4P3/8/8/8/6K1/8/4r2k w - - 0 1",
			uciMove:           "e7e8q",
			expected:          true,
			expectedPieceType: chess.Queen,
		},
		// --- Defended-piece cases (SEE filters out false sacrifices) ---
		{
			// White Bishop on f4 moves to g5 (empty), defended by White pawn on h4.
			// Black Knight on e6 can capture on g5 (Nxg5), but White recaptures hxg5.
			// Exchange: Bishop(300) for Knight(300) = even → not a sacrifice.
			name:              "bishop offering exchange defended by pawn — SEE equal, not a sacrifice",
			fen:               "7k/8/4n3/8/5B1P/8/8/6K1 w - - 0 1",
			uciMove:           "f4g5",
			expected:          false,
			expectedPieceType: chess.NoPieceType,
		},
		{
			// White Knight on c3 moves to d5 (empty), defended by White Knight on f4.
			// Black Bishop on b7 can capture (Bxd5), but White recaptures Nxd5.
			// Exchange: Knight(300) for Bishop(300) = even → not a sacrifice.
			name:              "knight to outpost defended by another knight — SEE equal, not a sacrifice",
			fen:               "7k/1b6/8/8/5N2/2N5/8/6K1 w - - 0 1",
			uciMove:           "c3d5",
			expected:          false,
			expectedPieceType: chess.NoPieceType,
		},
		{
			// White Rook on d3 moves to d6, backed by White Rook on d1.
			// Black Rook on d8 can capture (Rxd6), White recaptures (Rxd6).
			// Exchange: Rook(500) for Rook(500) = even → not a sacrifice.
			name:              "rook doubled on file — backed rook is not a sacrifice",
			fen:               "3r3k/8/8/8/8/3R4/8/3R2K1 w - - 0 1",
			uciMove:           "d3d6",
			expected:          false,
			expectedPieceType: chess.NoPieceType,
		},
		{
			// White Knight on c3 captures Black pawn on d5, defended by White pawn on e4.
			// After Nxd5: Black cxd5, White exd5. Net for White: captured pawn(100) + pawn(100) - Knight(300) = -100.
			// SEE = 200 > capturedValue(100) → still a sacrifice (White loses 100cp net).
			name:              "knight captures pawn with pawn-defender — partial sacrifice, still a sacrifice",
			fen:               "7k/8/2p5/3p4/4P3/2N5/8/6K1 w - - 0 1",
			uciMove:           "c3d5",
			expected:          true,
			expectedPieceType: chess.Knight,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			before, after, move := getTestPositions(t, tc.fen, tc.uciMove)
			result, pieceType := detectSacrifice(before, after, move)

			assert.Equal(t, tc.expected, result)
			assert.Equal(t, tc.expectedPieceType, pieceType)
		})
	}
}

// positionFromFEN parses fen and returns the resulting position.
// It fails the test immediately if the FEN is invalid.
func positionFromFEN(t *testing.T, fen string) *chess.Position {
	t.Helper()

	fenOpt, err := chess.FEN(fen)
	require.NoError(t, err, "invalid FEN")

	game := chess.NewGame(fenOpt)

	return game.Position()
}

func TestLeastValuableAttacker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		fen           string
		targetSquare  chess.Square
		expectNil     bool
		expectedValue int
	}{
		{
			// No Black piece can reach d5. King is too far away.
			name:         "no attacker available",
			fen:          "7k/8/8/3N4/8/8/8/4K3 b - - 0 1",
			targetSquare: chess.D5,
			expectNil:    true,
		},
		{
			// Only Black pawn on c6 can capture on d5.
			name:          "single pawn attacker",
			fen:           "7k/8/2p5/3N4/8/8/8/4K3 b - - 0 1",
			targetSquare:  chess.D5,
			expectedValue: pawnValue,
		},
		{
			// Black pawn on c6 (100) and Black Bishop on b7 (300) both attack d5.
			// Least valuable = pawn.
			name:          "pawn chosen over bishop",
			fen:           "7k/1b6/2p5/3N4/8/8/8/4K3 b - - 0 1",
			targetSquare:  chess.D5,
			expectedValue: pawnValue,
		},
		{
			// Black Bishop on b7 (300) and Black Rook on d8 (500) both attack d5.
			// Least valuable = Bishop.
			name:          "bishop chosen over rook",
			fen:           "3r3k/1b6/8/3N4/8/8/8/4K3 b - - 0 1",
			targetSquare:  chess.D5,
			expectedValue: bishopValue,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pos := positionFromFEN(t, tc.fen)
			move, value := leastValuableAttacker(pos, tc.targetSquare)

			if tc.expectNil {
				assert.Nil(t, move)
				assert.Equal(t, 0, value)
			} else {
				require.NotNil(t, move)
				assert.Equal(t, tc.expectedValue, value)
				assert.Equal(t, tc.targetSquare, move.S2())
			}
		})
	}
}

func TestStaticExchangeEval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fen          string
		targetSquare chess.Square
		targetValue  int
		expectedSEE  int
	}{
		{
			// No attacker can reach d5 → SEE = 0.
			name:         "no attacker — SEE is zero",
			fen:          "7k/8/8/3N4/8/8/8/4K3 b - - 0 1",
			targetSquare: chess.D5,
			targetValue:  knightValue,
			expectedSEE:  0,
		},
		{
			// Black pawn captures undefended Knight on d5 → SEE = 300.
			name:         "undefended knight — SEE equals knight value",
			fen:          "7k/8/2p5/3N4/8/8/8/4K3 b - - 0 1",
			targetSquare: chess.D5,
			targetValue:  knightValue,
			expectedSEE:  knightValue,
		},
		{
			// White Knight on d5, defended by White pawn on e4. Black pawn on c6.
			// cxd5, exd5 → exchange: pawn for Knight, then pawn for pawn.
			// SEE = max(0, 300 - max(0, 100 - 0)) = max(0, 300-100) = 200.
			name:         "knight defended by pawn — SEE is partial",
			fen:          "7k/8/2p5/3N4/4P3/8/8/4K3 b - - 0 1",
			targetSquare: chess.D5,
			targetValue:  knightValue,
			expectedSEE:  200,
		},
		{
			// White Knight on d5, defended by White Knight on f4.
			// Black Bishop on b7 captures Bxd5, White recaptures Nxd5.
			// SEE = max(0, 300 - max(0, 300 - 0)) = 0.
			name:         "knight defended by knight — SEE is zero (equal exchange)",
			fen:          "7k/1b6/8/3N4/5N2/8/8/4K3 b - - 0 1",
			targetSquare: chess.D5,
			targetValue:  knightValue,
			expectedSEE:  0,
		},
		{
			// White Rook on d6, defended by White Rook on d1. Black Rook on d8.
			// Rxd6, Rxd6 → Rook for Rook = even. SEE = 0.
			name:         "rook defended by rook — SEE is zero",
			fen:          "3r3k/8/3R4/8/8/8/8/3R2K1 b - - 0 1",
			targetSquare: chess.D6,
			targetValue:  rookValue,
			expectedSEE:  0,
		},
		{
			// White Queen on d5. Black pawn on c6 and Black Rook on d8 attack d5.
			// Least valuable: pawn(100). cxd5 gains 900. White has no defender.
			// SEE = 900.
			name:         "undefended queen — SEE equals queen value",
			fen:          "3r3k/8/2p5/3Q4/8/8/8/6K1 b - - 0 1",
			targetSquare: chess.D5,
			targetValue:  queenValue,
			expectedSEE:  queenValue,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pos := positionFromFEN(t, tc.fen)
			result := staticExchangeEval(pos, tc.targetSquare, tc.targetValue)

			assert.Equal(t, tc.expectedSEE, result)
		})
	}
}
