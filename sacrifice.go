package chessreview

import "github.com/corentings/chess/v2"

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

// leastValuableAttacker finds the legal move that captures on targetSquare using
// the least valuable piece of the side to move. For promotions the effective
// value is the promoted piece's value (the piece that will actually sit on the
// square after capturing). Returns (nil, 0) when no legal capture exists.
func leastValuableAttacker(pos *chess.Position, targetSquare chess.Square) (move *chess.Move, value int) {
	moves := pos.ValidMoves()
	board := pos.Board()

	bestIdx := -1
	bestValue := 0

	for i := range moves {
		if moves[i].S2() != targetSquare {
			continue
		}

		pieceType := board.Piece(moves[i].S1()).Type()
		if promo := moves[i].Promo(); promo != chess.NoPieceType {
			pieceType = promo
		}

		value := pieceTypeValue(pieceType)
		if bestIdx == -1 || value < bestValue {
			bestIdx = i
			bestValue = value
		}
	}

	if bestIdx == -1 {
		return nil, 0
	}

	return &moves[bestIdx], bestValue
}

// staticExchangeEval computes the material gain for the side to move when
// initiating or continuing a capture chain on targetSquare. targetValue is
// the value of the piece currently sitting on the square.
//
// The function uses a negamax formulation with the invariant max(0, …),
// which models each side's option to decline a recapture when it would
// lose material. A return value of 0 means either no attacker exists or
// capturing is not profitable for the side to move.
//
// Legal-move generation (Position.ValidMoves) is used at each level, so
// pinned pieces are correctly excluded and king safety is respected.
func staticExchangeEval(pos *chess.Position, targetSquare chess.Square, targetValue int) int {
	attacker, attackerValue := leastValuableAttacker(pos, targetSquare)
	if attacker == nil {
		return 0
	}

	// Simulate the capture.
	newPos := pos.Update(attacker)

	// The capturing side gains targetValue but puts attackerValue at risk.
	// The opponent may recapture (gaining attackerValue minus their own risk).
	// max(0, …) reflects that the opponent can decline the recapture.
	opponentGain := staticExchangeEval(newPos, targetSquare, attackerValue)

	gain := targetValue - opponentGain
	if gain < 0 {
		return 0
	}

	return gain
}

// detectSacrifice reports whether move constitutes a material sacrifice and,
// when it does, returns the piece type that was sacrificed.
//
// A sacrifice is detected when two conditions are both met:
//  1. The effective value of the piece arriving on the destination square
//     exceeds the value of any piece captured there (net material investment
//     is negative for the moving side if the opponent recaptures).
//  2. Static Exchange Evaluation (SEE) on the destination square shows that
//     recapturing is profitable for the opponent when the full capture chain
//     is considered. This filters out "false sacrifices" where the piece is
//     well-defended (e.g. a Knight on an outpost protected by a pawn).
//
// Returns (true, movedPieceType) when a sacrifice is detected, or
// (false, chess.NoPieceType) otherwise.
//
// beforePos is the position immediately before move is applied.
// afterPos is the position immediately after move is applied.
// Promotions are handled by substituting the promoted piece's value for the pawn.
// En passant is handled by using pawn value when the destination square is empty.
func detectSacrifice(beforePos, afterPos *chess.Position, move *chess.Move) (bool, chess.PieceType) {
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
		return false, chess.NoPieceType
	}

	// Use Static Exchange Evaluation (SEE) to determine whether recapturing
	// is actually profitable for the opponent once the full capture chain is
	// considered. If the SEE value for the opponent is at most capturedValue,
	// the moving side breaks even or gains material — the piece is sufficiently
	// defended and this is not a real sacrifice.
	seeValue := staticExchangeEval(afterPos, toSquare, movedValue)
	if seeValue > capturedValue {
		return true, movedPieceType
	}

	return false, chess.NoPieceType
}
