package chessreview

import (
	"context"
	"errors"
	"testing"

	"github.com/ksysoev/stockfish"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEngine is a test double for the chessEngine interface.
type mockEngine struct {
	newGameErr     error
	setPositionErr error
	goErr          error
	applyErr       error
	searchInfos    []stockfish.SearchInfo
	callCount      int
}

func (m *mockEngine) NewGame() error {
	return m.newGameErr
}

func (m *mockEngine) SetPosition(_ stockfish.Position) error {
	return m.setPositionErr
}

func (m *mockEngine) Go(_ context.Context, _ *stockfish.SearchParams) (<-chan stockfish.SearchInfo, error) {
	if m.goErr != nil {
		return nil, m.goErr
	}

	// Each call to Go consumes the next pair of infos (depth info + bestmove info).
	// callCount tracks how many analyzePosition calls have been made.
	batchSize := 2

	start := m.callCount * batchSize
	m.callCount++

	end := start + batchSize
	if end > len(m.searchInfos) {
		end = len(m.searchInfos)
	}

	batch := m.searchInfos[start:end]

	ch := make(chan stockfish.SearchInfo, len(batch))
	for i := range batch {
		ch <- batch[i]
	}

	close(ch)

	return ch, nil
}

func (m *mockEngine) Apply(_ ...stockfish.Option) error {
	return m.applyErr
}

func (m *mockEngine) Close() error {
	return nil
}

// makeDepthInfo returns a non-bestmove SearchInfo with the given centipawn score.
func makeDepthInfo(score int) stockfish.SearchInfo {
	return stockfish.SearchInfo{
		Score: stockfish.Score{Value: score, Type: stockfish.ScoreTypeCentipawns},
		Depth: 1,
	}
}

// makeMateInfo returns a non-bestmove SearchInfo with a mate-in-N score.
// Positive n means the side to move has a forced mate; negative means they are
// being mated.
func makeMateInfo(n int) stockfish.SearchInfo {
	return stockfish.SearchInfo{
		Score: stockfish.Score{Value: n, Type: stockfish.ScoreTypeMate},
		Depth: 1,
	}
}

// makeBestMoveInfo returns a bestmove SearchInfo.
func makeBestMoveInfo(bestMove string) stockfish.SearchInfo {
	return stockfish.SearchInfo{
		IsBestMove: true,
		BestMove:   bestMove,
	}
}

func TestReviewer_ReviewGame_TwoMoves(t *testing.T) {
	t.Parallel()

	const pgn = `[Event "Test"]
[Site "?"]
[Date "????.??.??"]
[Round "?"]
[White "White"]
[Black "Black"]
[Result "*"]

1. e4 e5 *`

	// analyzePosition is called 3 times total (N+1 for 2 moves):
	//   call 0 (initial position, white to move):  score=20, best=e2e4
	//   call 1 (after e4, black to move):          score=30, best=e7e5
	//   call 2 (after e5, white to move):          score=10, best=d2d4
	engine := &mockEngine{
		searchInfos: []stockfish.SearchInfo{
			makeDepthInfo(20), makeBestMoveInfo("e2e4"),
			makeDepthInfo(30), makeBestMoveInfo("e7e5"),
			makeDepthInfo(10), makeBestMoveInfo("d2d4"),
		},
	}

	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	reviews, err := r.ReviewGame(context.Background(), pgn)

	require.NoError(t, err)
	require.Len(t, reviews, 2)

	// White's e4: played == best → Best
	assert.Equal(t, "e2e4", reviews[0].PlayedMove)
	assert.Equal(t, "e2e4", reviews[0].BestMove)
	assert.Equal(t, "white", reviews[0].Color)
	assert.Equal(t, 1, reviews[0].MoveNumber)
	assert.Equal(t, Best, reviews[0].Classification)

	// Black's e5: played == best → Best
	assert.Equal(t, "e7e5", reviews[1].PlayedMove)
	assert.Equal(t, "e7e5", reviews[1].BestMove)
	assert.Equal(t, "black", reviews[1].Color)
	assert.Equal(t, 1, reviews[1].MoveNumber)
	assert.Equal(t, Best, reviews[1].Classification)
}

func TestReviewer_ReviewGame_InvalidPGN(t *testing.T) {
	t.Parallel()

	engine := &mockEngine{}
	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	_, err := r.ReviewGame(context.Background(), "not valid pgn!!!")

	require.Error(t, err)

	var pgnErr *ErrInvalidPGN

	assert.ErrorAs(t, err, &pgnErr)
}

func TestReviewer_ReviewGame_NewGameError(t *testing.T) {
	t.Parallel()

	const pgn = `[Event "Test"]
[Result "*"]

1. e4 e5 *`

	engine := &mockEngine{newGameErr: errors.New("engine died")}
	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	_, err := r.ReviewGame(context.Background(), pgn)

	require.Error(t, err)

	var engErr *ErrEngineFailure

	assert.ErrorAs(t, err, &engErr)
}

func TestReviewer_ReviewGame_EngineGoError(t *testing.T) {
	t.Parallel()

	const pgn = `[Event "Test"]
[Result "*"]

1. e4 e5 *`

	engine := &mockEngine{goErr: errors.New("search failed")}
	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	_, err := r.ReviewGame(context.Background(), pgn)

	require.Error(t, err)

	var engErr *ErrEngineFailure

	assert.ErrorAs(t, err, &engErr)
}

func TestNew_InvalidPath(t *testing.T) {
	t.Parallel()

	_, err := New("/nonexistent/path/to/stockfish")

	require.Error(t, err)

	var engErr *ErrEngineFailure

	assert.ErrorAs(t, err, &engErr)
}

func TestReviewer_ReviewGame_NoBestMove(t *testing.T) {
	t.Parallel()

	const pgn = `[Event "Test"]
[Result "*"]

1. e4 e5 *`

	// The mock closes the channel after a depth info but never sends a bestmove
	// line — analyzePosition must return ErrEngineFailure.
	engine := &mockEngine{
		searchInfos: []stockfish.SearchInfo{
			makeDepthInfo(20), // no makeBestMoveInfo — channel closes here
		},
	}

	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	_, err := r.ReviewGame(context.Background(), pgn)

	require.Error(t, err)

	var engErr *ErrEngineFailure

	assert.ErrorAs(t, err, &engErr)
	assert.Contains(t, engErr.Error(), "no best move")
}

func TestReviewer_ReviewGame_MateScore(t *testing.T) {
	t.Parallel()

	const pgn = `[Event "Test"]
[Result "*"]

1. e4 e5 *`

	// call 0 (initial, white to move):  mate-in-1 for white → sentinel +30000,
	//                                    but best move is d2d4 (not e2e4).
	// call 1 (after e4, black to move): cp score 0, best=e7e5
	// call 2 (after e5, white to move): cp score 0, best=d2d4
	//
	// White plays e4 but had a forced mate with d2d4 → delta = -0 - 30000 = -30000
	// loss = 30000 → Miss (threw away the forced mate).
	engine := &mockEngine{
		searchInfos: []stockfish.SearchInfo{
			makeMateInfo(1), makeBestMoveInfo("d2d4"), // best is d2d4, not e2e4
			makeDepthInfo(0), makeBestMoveInfo("e7e5"),
			makeDepthInfo(0), makeBestMoveInfo("d2d4"),
		},
	}

	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	reviews, err := r.ReviewGame(context.Background(), pgn)

	require.NoError(t, err)
	require.Len(t, reviews, 2)

	// White played e4 but had a forced mate with d2d4 — classified as Miss.
	assert.Equal(t, Miss, reviews[0].Classification)
	assert.Equal(t, mateScoreSentinel, reviews[0].ScoreBefore)
}

func TestNormalizeScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		score    stockfish.Score
		expected int
	}{
		{
			name:     "centipawn score passed through",
			score:    stockfish.Score{Type: stockfish.ScoreTypeCentipawns, Value: 42},
			expected: 42,
		},
		{
			name:     "negative centipawn score passed through",
			score:    stockfish.Score{Type: stockfish.ScoreTypeCentipawns, Value: -100},
			expected: -100,
		},
		{
			name:     "positive mate score maps to +sentinel",
			score:    stockfish.Score{Type: stockfish.ScoreTypeMate, Value: 3},
			expected: mateScoreSentinel,
		},
		{
			name:     "negative mate score maps to -sentinel",
			score:    stockfish.Score{Type: stockfish.ScoreTypeMate, Value: -2},
			expected: -mateScoreSentinel,
		},
		{
			name:     "mate-in-1 maps to +sentinel",
			score:    stockfish.Score{Type: stockfish.ScoreTypeMate, Value: 1},
			expected: mateScoreSentinel,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := normalizeScore(tc.score)
			assert.Equal(t, tc.expected, result)
		})
	}
}
