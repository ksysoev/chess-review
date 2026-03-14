package chessreview

import (
	"strings"

	"github.com/notnil/chess"
)

// moveInfo holds extracted data for a single half-move (ply).
type moveInfo struct {
	// UCIMove is the move in UCI long algebraic notation (e.g. "e2e4", "g1f3").
	UCIMove string
	// Color is "white" or "black".
	Color string
	// MoveNumber is the full-move number (1-indexed; increments after Black's move).
	MoveNumber int
}

// parsePGN parses a PGN string and returns an ordered slice of moveInfo for each
// half-move (ply) in the game.
//
// It returns ErrInvalidPGN when the PGN cannot be parsed or contains no moves.
func parsePGN(pgn string) ([]moveInfo, error) {
	reader := strings.NewReader(pgn)

	games, err := chess.GamesFromPGN(reader)
	if err != nil {
		return nil, &ErrInvalidPGN{Cause: err, Reason: err.Error()}
	}

	if len(games) == 0 {
		return nil, &ErrInvalidPGN{Reason: "no games found in PGN"}
	}

	game := games[0]
	positions := game.Positions()
	moves := game.Moves()

	if len(moves) == 0 {
		return nil, &ErrInvalidPGN{Reason: "game contains no moves"}
	}

	infos := make([]moveInfo, 0, len(moves))

	for i, move := range moves {
		pos := positions[i]

		color := "white"
		if pos.Turn() == chess.Black {
			color = "black"
		}

		// Full-move number: increments after Black plays.
		moveNumber := (i / 2) + 1 //nolint:mnd // dividing ply index by 2 to get full-move number is self-explanatory

		infos = append(infos, moveInfo{
			UCIMove:    moveToUCI(move),
			Color:      color,
			MoveNumber: moveNumber,
		})
	}

	return infos, nil
}

// moveToUCI converts a *chess.Move to UCI long algebraic notation.
// Promotion piece is appended as a lowercase letter when applicable.
func moveToUCI(m *chess.Move) string {
	from := m.S1().String()
	to := m.S2().String()

	promo := ""
	if p := m.Promo(); p != chess.NoPieceType {
		promo = strings.ToLower(p.String())
	}

	return from + to + promo
}
