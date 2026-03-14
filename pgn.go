package chessreview

import (
	"strconv"
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
	// IsSacrifice is true when the move gives up material that the opponent can
	// immediately recapture, making it a candidate for a Brilliant annotation.
	IsSacrifice bool
}

// gameInfo holds the parsed moves and the initial position FEN for the game.
type gameInfo struct {
	// InitialFEN is the Forsyth-Edwards Notation of the starting position.
	// For standard games this is the default starting FEN; for SetUp/FEN games
	// it reflects the custom starting position from the PGN header.
	InitialFEN string
	// Moves is the ordered list of half-moves extracted from the game.
	Moves []moveInfo
}

// parsePGN parses a PGN string and returns a gameInfo containing the initial
// position FEN and an ordered slice of moveInfo for each half-move (ply).
//
// It returns ErrInvalidPGN when the PGN cannot be parsed or contains no moves.
func parsePGN(pgn string) (gameInfo, error) {
	reader := strings.NewReader(pgn)

	games, err := chess.GamesFromPGN(reader)
	if err != nil {
		return gameInfo{}, &ErrInvalidPGN{Cause: err, Reason: err.Error()}
	}

	if len(games) == 0 {
		return gameInfo{}, &ErrInvalidPGN{Reason: "no games found in PGN"}
	}

	game := games[0]
	positions := game.Positions()
	moves := game.Moves()

	if len(moves) == 0 {
		return gameInfo{}, &ErrInvalidPGN{Reason: "game contains no moves"}
	}

	// positions[0] is always the initial position. Its String() method returns
	// the full FEN, which includes the full-move number (field 6) and the
	// side to move (field 2).
	initialPos := positions[0]
	initialFEN := initialPos.String()

	startMoveNum, startBlack := parseFENMoveContext(initialFEN)

	infos := make([]moveInfo, 0, len(moves))

	for i, move := range moves {
		pos := positions[i]

		color := "white"
		if pos.Turn() == chess.Black {
			color = "black"
		}

		// Compute the full-move number correctly regardless of the starting
		// position. startBlack is 1 when the game begins with Black to move,
		// which shifts the ply-to-move mapping by one.
		//nolint:mnd // arithmetic: (ply + black-offset) / 2 gives full-move number
		moveNumber := startMoveNum + (i+startBlack)/2

		infos = append(infos, moveInfo{
			UCIMove:     moveToUCI(move),
			Color:       color,
			MoveNumber:  moveNumber,
			IsSacrifice: detectSacrifice(positions[i], positions[i+1], move),
		})
	}

	return gameInfo{InitialFEN: initialFEN, Moves: infos}, nil
}

// parseFENMoveContext extracts the full-move number and starting-side offset
// from a FEN string. It returns (startMoveNum, startBlack) where startBlack is
// 1 if Black is to move in the FEN (so that ply indices map correctly to
// full-move numbers) and 0 otherwise. On any parse error the function returns
// safe defaults (1, 0) corresponding to the standard starting position.
func parseFENMoveContext(fen string) (startMoveNum, startBlack int) {
	parts := strings.Fields(fen)

	const fenFields = 6

	if len(parts) < fenFields {
		return 1, 0
	}

	moveNum, err := strconv.Atoi(parts[5])
	if err != nil || moveNum < 1 {
		moveNum = 1
	}

	blackOffset := 0
	if parts[1] == "b" {
		blackOffset = 1
	}

	return moveNum, blackOffset
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
