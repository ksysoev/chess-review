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

	moves, err := parsePGN(scholarsMate)

	require.NoError(t, err)
	assert.Len(t, moves, 7)

	// First move: e2e4 by White, move number 1
	assert.Equal(t, "e2e4", moves[0].UCIMove)
	assert.Equal(t, "white", moves[0].Color)
	assert.Equal(t, 1, moves[0].MoveNumber)

	// Second move: e7e5 by Black, move number 1
	assert.Equal(t, "e7e5", moves[1].UCIMove)
	assert.Equal(t, "black", moves[1].Color)
	assert.Equal(t, 1, moves[1].MoveNumber)

	// Third move: Qh5 (d1h5) by White, move number 2
	assert.Equal(t, "white", moves[2].Color)
	assert.Equal(t, 2, moves[2].MoveNumber)
}

func TestParsePGN_TwoMoves(t *testing.T) {
	t.Parallel()

	moves, err := parsePGN(twoMoveGame)

	require.NoError(t, err)
	assert.Len(t, moves, 2)

	assert.Equal(t, "e2e4", moves[0].UCIMove)
	assert.Equal(t, "white", moves[0].Color)

	assert.Equal(t, "e7e5", moves[1].UCIMove)
	assert.Equal(t, "black", moves[1].Color)
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
