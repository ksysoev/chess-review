package chessreview

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const scholarsMate = `[Event "Test"]
[Site "?"]
[Date "????.??.??"]
[Round "?"]
[White "White"]
[Black "Black"]
[Result "1-0"]

1. e4 e5 2. Qh5 Nc6 3. Bc4 Nf6 4. Qxf7# 1-0`

const twoMoveGame = `[Event "Test"]
[Site "?"]
[Date "????.??.??"]
[Round "?"]
[White "White"]
[Black "Black"]
[Result "*"]

1. e4 e5 *`

func TestParsePGN_ValidGame(t *testing.T) {
	t.Parallel()

	gi, err := parsePGN(scholarsMate)

	require.NoError(t, err)
	assert.Len(t, gi.Moves, 7)

	// First move: e2e4 by White, move number 1
	assert.Equal(t, "e2e4", gi.Moves[0].UCIMove)
	assert.Equal(t, "white", gi.Moves[0].Color)
	assert.Equal(t, 1, gi.Moves[0].MoveNumber)

	// Second move: e7e5 by Black, move number 1
	assert.Equal(t, "e7e5", gi.Moves[1].UCIMove)
	assert.Equal(t, "black", gi.Moves[1].Color)
	assert.Equal(t, 1, gi.Moves[1].MoveNumber)

	// Third move: Qh5 (d1h5) by White, move number 2
	assert.Equal(t, "white", gi.Moves[2].Color)
	assert.Equal(t, 2, gi.Moves[2].MoveNumber)
}

func TestParsePGN_TwoMoves(t *testing.T) {
	t.Parallel()

	gi, err := parsePGN(twoMoveGame)

	require.NoError(t, err)
	assert.Len(t, gi.Moves, 2)

	assert.Equal(t, "e2e4", gi.Moves[0].UCIMove)
	assert.Equal(t, "white", gi.Moves[0].Color)

	assert.Equal(t, "e7e5", gi.Moves[1].UCIMove)
	assert.Equal(t, "black", gi.Moves[1].Color)
}

func TestParsePGN_InvalidPGN(t *testing.T) {
	t.Parallel()

	_, err := parsePGN("this is not valid pgn at all !!!")

	require.Error(t, err)

	var pgnErr *ErrInvalidPGN

	assert.ErrorAs(t, err, &pgnErr)
}

func TestParsePGN_EmptyString(t *testing.T) {
	t.Parallel()

	_, err := parsePGN("")

	require.Error(t, err)

	var pgnErr *ErrInvalidPGN

	assert.ErrorAs(t, err, &pgnErr)
}

func TestParsePGN_NoMoves(t *testing.T) {
	t.Parallel()

	pgn := `[Event "Empty"]
[Result "*"]

*`

	_, err := parsePGN(pgn)

	require.Error(t, err)

	var pgnErr *ErrInvalidPGN

	assert.ErrorAs(t, err, &pgnErr)
	assert.Contains(t, pgnErr.Error(), "no moves")
}

// promotionPGN is a minimal game from a FEN where white promotes e7→e8=Q.
// The FEN places a white pawn on e7 with kings on e1 and a8.
const promotionPGN = `[Event "Test"]
[Site "?"]
[Date "????.??.??"]
[Round "?"]
[White "White"]
[Black "Black"]
[Result "1-0"]
[SetUp "1"]
[FEN "k7/4P3/8/8/8/8/8/4K3 w - - 0 1"]

1. e8=Q 1-0`

// castlingPGN is a standard opening where white castles kingside on move 4.
const castlingPGN = `[Event "Test"]
[Site "?"]
[Date "????.??.??"]
[Round "?"]
[White "White"]
[Black "Black"]
[Result "*"]

1. e4 e5 2. Nf3 Nc6 3. Bc4 Bc5 4. O-O *`

func TestMoveToUCI_Promotion(t *testing.T) {
	t.Parallel()

	gi, err := parsePGN(promotionPGN)

	require.NoError(t, err)
	require.Len(t, gi.Moves, 1)

	// e7→e8 with queen promotion must produce "e7e8q"
	assert.Equal(t, "e7e8q", gi.Moves[0].UCIMove)
	assert.Equal(t, "white", gi.Moves[0].Color)
	assert.Equal(t, 1, gi.Moves[0].MoveNumber)
}

func TestMoveToUCI_Castling(t *testing.T) {
	t.Parallel()

	gi, err := parsePGN(castlingPGN)

	require.NoError(t, err)
	// 7 half-moves: e4, e5, Nf3, Nc6, Bc4, Bc5, O-O
	require.Len(t, gi.Moves, 7)

	// O-O (kingside castling) must be represented as e1g1 in UCI
	assert.Equal(t, "e1g1", gi.Moves[6].UCIMove)
	assert.Equal(t, "white", gi.Moves[6].Color)
	assert.Equal(t, 4, gi.Moves[6].MoveNumber)
}

// fenMoveNumberPGN is a game starting from move 5 with White to move.
// The FEN full-move counter is 5, so the first move should be numbered 5.
const fenMoveNumberPGN = `[Event "Test"]
[Site "?"]
[Date "????.??.??"]
[Round "?"]
[White "White"]
[Black "Black"]
[Result "*"]
[SetUp "1"]
[FEN "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 5"]

5. e4 e5 *`

// fenBlackFirstPGN starts with Black to move at full-move 3.
// The first ply is Black's, so its MoveNumber must be 3, and White's
// subsequent move must be 4.
const fenBlackFirstPGN = `[Event "Test"]
[Site "?"]
[Date "????.??.??"]
[Round "?"]
[White "White"]
[Black "Black"]
[Result "*"]
[SetUp "1"]
[FEN "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq - 0 3"]

3... e5 4. Nf3 *`

func TestParsePGN_FENMoveNumber(t *testing.T) {
	t.Parallel()

	gi, err := parsePGN(fenMoveNumberPGN)

	require.NoError(t, err)
	require.Len(t, gi.Moves, 2)

	// White's e4 at full-move 5
	assert.Equal(t, "e2e4", gi.Moves[0].UCIMove)
	assert.Equal(t, "white", gi.Moves[0].Color)
	assert.Equal(t, 5, gi.Moves[0].MoveNumber)

	// Black's e5 at full-move 5
	assert.Equal(t, "e7e5", gi.Moves[1].UCIMove)
	assert.Equal(t, "black", gi.Moves[1].Color)
	assert.Equal(t, 5, gi.Moves[1].MoveNumber)
}

func TestParsePGN_FENBlackFirst(t *testing.T) {
	t.Parallel()

	gi, err := parsePGN(fenBlackFirstPGN)

	require.NoError(t, err)
	require.Len(t, gi.Moves, 2)

	// Black's e5 at full-move 3
	assert.Equal(t, "e7e5", gi.Moves[0].UCIMove)
	assert.Equal(t, "black", gi.Moves[0].Color)
	assert.Equal(t, 3, gi.Moves[0].MoveNumber)

	// White's Nf3 at full-move 4
	assert.Equal(t, "g1f3", gi.Moves[1].UCIMove)
	assert.Equal(t, "white", gi.Moves[1].Color)
	assert.Equal(t, 4, gi.Moves[1].MoveNumber)
}

func TestParsePGN_FENInitialFEN(t *testing.T) {
	t.Parallel()

	// Standard game should use the standard starting FEN.
	gi, err := parsePGN(twoMoveGame)

	require.NoError(t, err)
	assert.NotEmpty(t, gi.InitialFEN)
	// The standard starting FEN ends with "0 1" (halfmove clock 0, fullmove 1).
	assert.Contains(t, gi.InitialFEN, " 0 1")

	// FEN game should preserve the custom starting FEN.
	giFEN, err := parsePGN(fenMoveNumberPGN)

	require.NoError(t, err)
	assert.Contains(t, giFEN.InitialFEN, " 0 5")
}

func TestParsePGN_PlayerNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pgn       string
		wantWhite string
		wantBlack string
	}{
		{
			name:      "names present in standard tags",
			pgn:       scholarsMate,
			wantWhite: "White",
			wantBlack: "Black",
		},
		{
			name: "names absent — empty strings returned",
			pgn: `[Event "Test"]
[Result "*"]

1. e4 e5 *`,
			wantWhite: "",
			wantBlack: "",
		},
		{
			name: "custom player names",
			pgn: `[Event "Rapid"]
[White "Alice"]
[Black "Bob"]
[Result "*"]

1. d4 d5 *`,
			wantWhite: "Alice",
			wantBlack: "Bob",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gi, err := parsePGN(tt.pgn)

			require.NoError(t, err)
			assert.Equal(t, tt.wantWhite, gi.WhitePlayer)
			assert.Equal(t, tt.wantBlack, gi.BlackPlayer)
		})
	}
}

// italianGamePGN is the Italian Game opening — a well-known ECO line (C50).
// All five half-moves (e4, e5, Nf3, Nc6, Bc4) should be tagged as book moves.
const italianGamePGN = `[Event "Test"]
[Site "?"]
[Date "????.??.??"]
[Round "?"]
[White "White"]
[Black "Black"]
[Result "*"]

1. e4 e5 2. Nf3 Nc6 3. Bc4 *`

func TestParsePGN_BookMoves_Detected(t *testing.T) {
	t.Parallel()

	gi, err := parsePGN(italianGamePGN)

	require.NoError(t, err)
	require.Len(t, gi.Moves, 5)

	// All five moves of the Italian Game opening should be flagged as book moves.
	for i, mv := range gi.Moves {
		assert.True(t, mv.IsBook, "expected move %d (%s) to be a book move", i, mv.UCIMove)
	}
}

func TestParsePGN_BookMoves_NonBookAfterTheory(t *testing.T) {
	t.Parallel()

	// Scholar's Mate: 1.e4 e5 2.Qh5 Nc6 3.Bc4 Nf6 4.Qxf7#
	// e4 (index 0), e5 (index 1), and Qh5 (index 2) are ECO theory
	// (C20 King's Pawn Game: Wayward Queen Attack). Nc6 (index 3) is the
	// first deviation — no ECO line continues with Nc6 after Qh5.
	gi, err := parsePGN(scholarsMate)

	require.NoError(t, err)
	require.Len(t, gi.Moves, 7)

	// e4, e5, and Qh5 are all recognised opening theory.
	assert.True(t, gi.Moves[0].IsBook, "e4 (move 0) should be a book move")
	assert.True(t, gi.Moves[1].IsBook, "e5 (move 1) should be a book move")
	assert.True(t, gi.Moves[2].IsBook, "Qh5 (move 2) should be a book move (Wayward Queen Attack)")

	// Nc6 (index 3) deviates from theory — must NOT be flagged as book.
	assert.False(t, gi.Moves[3].IsBook, "Nc6 (move 3) must not be a book move after theory ends")

	// All subsequent moves must also not be book once theory ends.
	for i := 4; i < len(gi.Moves); i++ {
		assert.False(t, gi.Moves[i].IsBook, "move %d (%s) must not be a book move after theory ends", i, gi.Moves[i].UCIMove)
	}
}

func TestParsePGN_OpeningDetected(t *testing.T) {
	t.Parallel()

	gi, err := parsePGN(italianGamePGN)

	require.NoError(t, err)
	assert.Equal(t, "C50", gi.OpeningCode, "expected ECO code C50 for Italian Game")
	assert.Equal(t, "Italian Game", gi.OpeningTitle, "expected opening title 'Italian Game'")
}

func TestParsePGN_NoOpeningForNonStandardGame(t *testing.T) {
	t.Parallel()

	// A game starting from a custom FEN far into the endgame will not match any
	// ECO line because the opening book only covers standard starting positions.
	gi, err := parsePGN(promotionPGN)

	require.NoError(t, err)
	// The promotion game starts from a custom FEN — no opening should be detected.
	assert.Empty(t, gi.OpeningCode)
	assert.Empty(t, gi.OpeningTitle)
	// The single promotion move cannot be a book move.
	assert.False(t, gi.Moves[0].IsBook)
}
