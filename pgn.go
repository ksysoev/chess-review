package chessreview

import (
	"strconv"
	"strings"
	"sync"

	"github.com/corentings/chess/v2"
	openings "github.com/ksysoev/chess-openings"
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
	// SacrificedPieceType is the type of the piece that was sacrificed when
	// IsSacrifice is true, and chess.NoPieceType otherwise.
	SacrificedPieceType chess.PieceType
	// IsBook is true when the move is part of a known ECO opening line.
	// Book moves are not judged by engine evaluation and are excluded from
	// accuracy calculations.
	IsBook bool
}

// gameInfo holds the parsed moves and the initial position FEN for the game.
type gameInfo struct {
	// InitialFEN is the Forsyth-Edwards Notation of the starting position.
	// For standard games this is the default starting FEN; for SetUp/FEN games
	// it reflects the custom starting position from the PGN header.
	InitialFEN string
	// WhitePlayer is the name of the player with the white pieces, parsed from
	// the PGN White tag. Empty string when the tag is absent.
	WhitePlayer string
	// BlackPlayer is the name of the player with the black pieces, parsed from
	// the PGN Black tag. Empty string when the tag is absent.
	BlackPlayer string
	// OpeningCode is the ECO code of the detected opening (e.g. "C50").
	// Empty string when no opening was detected.
	OpeningCode string
	// OpeningTitle is the full name of the detected opening (e.g. "Italian Game").
	// Empty string when no opening was detected.
	OpeningTitle string
	// Moves is the ordered list of half-moves extracted from the game.
	Moves []moveInfo
}

// parsePGN parses a PGN string and returns a gameInfo containing the initial
// position FEN and an ordered slice of moveInfo for each half-move (ply).
//
// It returns ErrInvalidPGN when the PGN cannot be parsed or contains no moves.
func parsePGN(pgn string) (gameInfo, error) {
	reader := strings.NewReader(pgn)

	scanner := chess.NewScanner(reader)

	if !scanner.HasNext() {
		return gameInfo{}, &ErrInvalidPGN{Reason: "no games found in PGN"}
	}

	game, err := scanner.ParseNext()
	if err != nil {
		return gameInfo{}, &ErrInvalidPGN{Cause: err, Reason: err.Error()}
	}

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

	whiteName := tagValue(game, "White")
	blackName := tagValue(game, "Black")

	// Opening detection is only meaningful for games that start from the
	// standard position; custom FEN games are skipped entirely.
	detectOpenings := strings.HasPrefix(initialFEN, standardStartFEN)

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

		isSacrifice, sacrificedPieceType := detectSacrifice(positions[i], positions[i+1], move)

		infos = append(infos, moveInfo{
			UCIMove:             moveToUCI(move),
			Color:               color,
			MoveNumber:          moveNumber,
			IsSacrifice:         isSacrifice,
			SacrificedPieceType: sacrificedPieceType,
		})
	}

	// Identify the opening by matching board positions against the Lichess
	// opening database (~3,500 named openings). The position-based lookup
	// handles transpositions: if a game reaches a known opening position via
	// a non-standard move order it is still correctly identified.
	// All moves up to and including the deepest matched ply are marked as
	// book moves and excluded from engine-based classification.
	var openingCode, openingTitle string

	if detectOpenings {
		uciMoves := make([]string, len(infos))
		for i, info := range infos {
			uciMoves[i] = info.UCIMove
		}

		result, err := openingBook().Classify(uciMoves)
		if err == nil && result.Opening != nil {
			openingCode = result.Opening.ECO
			openingTitle = result.Opening.Name

			for i := range result.Ply {
				infos[i].IsBook = true
			}
		}
	}

	return gameInfo{
		InitialFEN:   initialFEN,
		WhitePlayer:  whiteName,
		BlackPlayer:  blackName,
		OpeningCode:  openingCode,
		OpeningTitle: openingTitle,
		Moves:        infos,
	}, nil
}

// standardStartFEN is the FEN for the standard chess starting position.
// ECO opening detection is only meaningful when a game begins from this position.
const standardStartFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq -"

var (
	openingBookOnce sync.Once
	openingBookInst *openings.Book
)

// openingBook returns a lazily-initialized singleton of the Lichess opening
// book (~3,500 named openings). The book is created once on first access and
// reused across all subsequent calls, avoiding repeated construction of the
// position map and move trie.
func openingBook() *openings.Book {
	openingBookOnce.Do(func() {
		openingBookInst = openings.New()
	})

	return openingBookInst
}

// tagValue returns the value of a PGN tag by name, or an empty string if the
// tag is absent. The chess.Game.GetTagPair method returns an empty string when missing.
func tagValue(g *chess.Game, tag string) string {
	return g.GetTagPair(tag)
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
