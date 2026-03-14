package chessreview

import "github.com/notnil/chess"

// Centipawn values for each piece type used in sacrifice detection.
// These are standard material values; King is given a high sentinel value
// because it acts as a recapturing piece without itself being capturable.
const (
	pawnValue   = 100
	knightValue = 300
	bishopValue = 300
	rookValue   = 500
	queenValue  = 900
	kingValue   = 10_000
)

// pieceTypeValues maps chess piece types to their centipawn values.
var pieceTypeValues = map[chess.PieceType]int{
	chess.Pawn:   pawnValue,
	chess.Knight: knightValue,
	chess.Bishop: bishopValue,
	chess.Rook:   rookValue,
	chess.Queen:  queenValue,
	chess.King:   kingValue,
}

// pieceTypeValue returns the centipawn value for pt.
// Returns 0 for chess.NoPieceType.
func pieceTypeValue(pt chess.PieceType) int {
	return pieceTypeValues[pt]
}

// detectSacrifice reports whether move constitutes a material sacrifice.
//
// A sacrifice is detected when two conditions are both met:
//  1. The effective value of the piece arriving on the destination square
//     exceeds the value of any piece captured there (net material investment
//     is negative for the moving side if the opponent recaptures).
//  2. The opponent has at least one legal recapture on that square in afterPos.
//
// beforePos is the position immediately before move is applied.
// afterPos is the position immediately after move is applied.
// Promotions are handled by substituting the promoted piece's value for the pawn.
// En passant is handled by using pawn value when the destination square is empty.
func detectSacrifice(beforePos, afterPos *chess.Position, move *chess.Move) bool {
	board := beforePos.Board()
	toSquare := move.S2()

	// Determine the effective value of the piece landing on toSquare.
	// For promotions the arriving piece is the promoted type, not a pawn.
	movedPieceType := board.Piece(move.S1()).Type()
	if promo := move.Promo(); promo != chess.NoPieceType {
		movedPieceType = promo
	}

	movedValue := pieceTypeValue(movedPieceType)

	// Value of what was captured on the destination square (0 for non-captures).
	capturedValue := 0

	if move.HasTag(chess.Capture) {
		capturedPiece := board.Piece(toSquare)
		if capturedPiece != chess.NoPiece {
			capturedValue = pieceTypeValue(capturedPiece.Type())
		} else {
			// En passant: the captured pawn is not on the destination square.
			capturedValue = pieceTypeValue(chess.Pawn)
		}
	}

	// Not a sacrifice when the immediate material exchange is even or in our favour.
	if capturedValue >= movedValue {
		return false
	}

	// A sacrifice requires the opponent to have at least one recapture available.
	for _, m := range afterPos.ValidMoves() {
		if m.S2() == toSquare {
			return true
		}
	}

	return false
}
