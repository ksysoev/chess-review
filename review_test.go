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

	// analyzePosition is called 4 times total (before/after × 2 moves).
	// Each call gets a depth info followed by a bestmove info.
	// Score perspective:
	//   call 0 (before e4, white to move): score=20, best=e2e4
	//   call 1 (after e4, black to move):  score=30, best=e7e5
	//   call 2 (before e5, black to move): score=25, best=e7e5
	//   call 3 (after e5, white to move):  score=10, best=d2d4
	engine := &mockEngine{
		searchInfos: []stockfish.SearchInfo{
			makeDepthInfo(20), makeBestMoveInfo("e2e4"),
			makeDepthInfo(30), makeBestMoveInfo("e7e5"),
			makeDepthInfo(25), makeBestMoveInfo("e7e5"),
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
