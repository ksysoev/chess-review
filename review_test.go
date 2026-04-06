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

// makeBoundInfo returns a non-bestmove SearchInfo with the given centipawn score
// and an explicit score bound (e.g. ScoreBoundLower or ScoreBoundUpper).
func makeBoundInfo(score int, bound stockfish.ScoreBound) stockfish.SearchInfo {
	return stockfish.SearchInfo{
		Score: stockfish.Score{Value: score, Type: stockfish.ScoreTypeCentipawns, Bound: bound},
		Depth: 1,
	}
}

// makeExactInfo returns a non-bestmove SearchInfo with an exact centipawn score
// (ScoreBoundExact, which is the zero value / empty string).
func makeExactInfo(score int) stockfish.SearchInfo {
	return stockfish.SearchInfo{
		Score: stockfish.Score{Value: score, Type: stockfish.ScoreTypeCentipawns, Bound: stockfish.ScoreBoundExact},
		Depth: 1,
	}
}

// makeMultiPVInfo returns a non-bestmove SearchInfo for MultiPV mode with the
// given PV index, exact centipawn score, and top move in the PV list.
func makeMultiPVInfo(pvIdx, score int, move string) stockfish.SearchInfo {
	return stockfish.SearchInfo{
		MultiPV: pvIdx,
		Score:   stockfish.Score{Value: score, Type: stockfish.ScoreTypeCentipawns, Bound: stockfish.ScoreBoundExact},
		PV:      []string{move},
		Depth:   1,
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

	// White's e4: ECO book move → Book (book classification takes priority).
	assert.Equal(t, "e2e4", reviews[0].PlayedMove)
	assert.Equal(t, "e2e4", reviews[0].TopMoves[0].Move)
	assert.Equal(t, "white", reviews[0].Color)
	assert.Equal(t, 1, reviews[0].MoveNumber)
	assert.Equal(t, Book, reviews[0].Classification)

	// Black's e5: ECO book move → Book.
	assert.Equal(t, "e7e5", reviews[1].PlayedMove)
	assert.Equal(t, "e7e5", reviews[1].TopMoves[0].Move)
	assert.Equal(t, "black", reviews[1].Color)
	assert.Equal(t, 1, reviews[1].MoveNumber)
	assert.Equal(t, Book, reviews[1].Classification)
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

func TestReviewer_ZeroValue_ReviewGame(t *testing.T) {
	t.Parallel()

	var r Reviewer

	_, err := r.ReviewGame(context.Background(), "1. e4 e5 *")

	require.Error(t, err)

	var engErr *ErrEngineFailure

	assert.ErrorAs(t, err, &engErr)
	assert.Contains(t, engErr.Error(), "not initialized")
}

func TestReviewer_ZeroValue_Close(t *testing.T) {
	t.Parallel()

	var r Reviewer

	err := r.Close()

	require.Error(t, err)

	var engErr *ErrEngineFailure

	assert.ErrorAs(t, err, &engErr)
	assert.Contains(t, engErr.Error(), "not initialized")
}

func TestNew_InvalidOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		opt    Option
		errMsg string
	}{
		{
			name:   "zero depth",
			opt:    WithDepth(0),
			errMsg: "invalid depth",
		},
		{
			name:   "negative depth",
			opt:    WithDepth(-1),
			errMsg: "invalid depth",
		},
		{
			name:   "zero threads",
			opt:    WithThreads(0),
			errMsg: "invalid threads",
		},
		{
			name:   "negative threads",
			opt:    WithThreads(-5),
			errMsg: "invalid threads",
		},
		{
			name:   "zero hash",
			opt:    WithHash(0),
			errMsg: "invalid hash size",
		},
		{
			name:   "negative hash",
			opt:    WithHash(-16),
			errMsg: "invalid hash size",
		},
		{
			name:   "zero top moves",
			opt:    WithTopMoves(0),
			errMsg: "invalid top moves",
		},
		{
			name:   "negative top moves",
			opt:    WithTopMoves(-1),
			errMsg: "invalid top moves",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// New() validates options before attempting to start the engine,
			// so it must return an ErrEngineFailure even with a nonexistent path.
			_, err := New("/nonexistent/stockfish", tc.opt)

			require.Error(t, err)

			var engErr *ErrEngineFailure

			assert.ErrorAs(t, err, &engErr)
			assert.Contains(t, err.Error(), tc.errMsg)
		})
	}
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

	// Use a custom FEN position (not in the ECO book) so that moves are NOT
	// classified as Book, allowing the Miss classification to be tested.
	// Position: white rook on a1, kings on e1/e8, white to move from move 10.
	// The FEN game is off-book, so engine scores drive classification.
	const pgn = `[Event "Test"]
[Result "*"]
[SetUp "1"]
[FEN "4k3/8/8/8/8/8/8/R3K3 w Q - 0 10"]

10. Ra8+ Ke7 *`

	// call 0 (initial, white to move):  mate-in-1 for white → sentinel +30000,
	//                                    but best move is a1a8 (Ra8+, not the played move).
	//                                    We simulate: best is a1a3, played is a1a8.
	// call 0: mate-in-1, best = a1a3 (not the played a1a8) → White misses the mate.
	// call 1 (after Ra8+, black to move): cp score 0, best=e8e7
	// call 2 (after Ke7, white to move):  cp score 0, best=a8a7
	//
	// White plays Ra8+ but had a "forced mate" with a1a3 → Miss.
	engine := &mockEngine{
		searchInfos: []stockfish.SearchInfo{
			makeMateInfo(1), makeBestMoveInfo("a1a3"), // best is a1a3, not a1a8
			makeDepthInfo(0), makeBestMoveInfo("e8e7"),
			makeDepthInfo(0), makeBestMoveInfo("a8a7"),
		},
	}

	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	reviews, err := r.ReviewGame(context.Background(), pgn)

	require.NoError(t, err)
	require.Len(t, reviews, 2)

	// White played Ra8+ but had a forced mate with a1a3 — classified as Miss.
	assert.Equal(t, Miss, reviews[0].Classification)
	assert.Equal(t, mateScoreSentinel, reviews[0].ScoreBefore)

	// MateInBefore on white's move: mate-in-1 was available before white moved.
	require.NotNil(t, reviews[0].MateInBefore)
	assert.Equal(t, 1, *reviews[0].MateInBefore)

	// After Ra8+ the engine reported cp score 0 (no forced mate) so MateInAfter is nil.
	assert.Nil(t, reviews[0].MateInAfter)

	// Black's move: no forced mate in the position before black moves.
	assert.Nil(t, reviews[1].MateInBefore)
	assert.Nil(t, reviews[1].MateInAfter)
}

func TestReviewer_ReviewGame_MateInNegative(t *testing.T) {
	t.Parallel()

	const pgn = `[Event "Test"]
[Result "*"]

1. e4 e5 *`

	// call 0 (initial): being mated in 2 (opponent has forced mate).
	// call 1 (after e4): cp score 0.
	// call 2 (after e5): cp score 0.
	engine := &mockEngine{
		searchInfos: []stockfish.SearchInfo{
			makeMateInfo(-2), makeBestMoveInfo("e2e4"),
			makeDepthInfo(0), makeBestMoveInfo("e7e5"),
			makeDepthInfo(0), makeBestMoveInfo("d2d4"),
		},
	}

	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	reviews, err := r.ReviewGame(context.Background(), pgn)

	require.NoError(t, err)
	require.Len(t, reviews, 2)

	// White's move: being mated in 2 before moving.
	require.NotNil(t, reviews[0].MateInBefore)
	assert.Equal(t, -2, *reviews[0].MateInBefore)

	// After e4 the engine reported cp score 0 — no forced mate in the resulting position.
	assert.Nil(t, reviews[0].MateInAfter)

	// Black's move: no forced mate before or after.
	assert.Nil(t, reviews[1].MateInBefore)
	assert.Nil(t, reviews[1].MateInAfter)
}

func TestReviewer_ReviewGame_NoMateIn(t *testing.T) {
	t.Parallel()

	const pgn = `[Event "Test"]
[Result "*"]

1. e4 e5 *`

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

	assert.Nil(t, reviews[0].MateInBefore)
	assert.Nil(t, reviews[0].MateInAfter)
	assert.Nil(t, reviews[1].MateInBefore)
	assert.Nil(t, reviews[1].MateInAfter)
}

func TestReviewer_ReviewGame_MateInAfter(t *testing.T) {
	t.Parallel()

	const pgn = `[Event "Test"]
[Result "*"]

1. e4 e5 *`

	// call 0 (initial, white to move):  cp score 0, best=e2e4
	// call 1 (after e4, black to move): mate-in-3 for black (opponent) → from
	//                                    white's frame MateInAfter = -3
	// call 2 (after e5, white to move): cp score 0, best=d2d4
	engine := &mockEngine{
		searchInfos: []stockfish.SearchInfo{
			makeDepthInfo(0), makeBestMoveInfo("e2e4"),
			makeMateInfo(3), makeBestMoveInfo("e7e5"),
			makeDepthInfo(0), makeBestMoveInfo("d2d4"),
		},
	}

	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	reviews, err := r.ReviewGame(context.Background(), pgn)

	require.NoError(t, err)
	require.Len(t, reviews, 2)

	// White's move: no forced mate before; opponent has mate-in-3 after → -3 from white's POV.
	assert.Nil(t, reviews[0].MateInBefore)
	require.NotNil(t, reviews[0].MateInAfter)
	assert.Equal(t, -3, *reviews[0].MateInAfter)

	// Black's move: the MateInBefore is the engine's mate-in-3 (from black's POV, positive).
	require.NotNil(t, reviews[1].MateInBefore)
	assert.Equal(t, 3, *reviews[1].MateInBefore)
	assert.Nil(t, reviews[1].MateInAfter)
}

func TestReviewer_ReviewGame_BrilliantMove(t *testing.T) {
	t.Parallel()

	// Use a custom FEN to set up a position that is off-book (not in the ECO
	// database), allowing the Brilliant classification to be exercised.
	//
	// Position after 1.e4 e5 2.Nf3 Nc6 3.Bc4 Bc5 (Italian Game) with an extra
	// pawn twist: we start from a FEN where it's White's turn to play b4 (the
	// Evans Gambit pawn push) but the game is flagged as a SetUp position so
	// no ECO book applies.
	//
	// FEN: Italian Game position after 3...Bc5, white to move.
	// rnbqk2r/pppp1ppp/2n2n2/2b1p3/2B1P3/5N2/PPPP1PPP/RNBQK2R w KQkq - 4 4
	// Wait — that has Black's knight on f6, which is not the Italian after 3.Bc4 Bc5.
	// Italian after 1.e4 e5 2.Nf3 Nc6 3.Bc4 Bc5: r1bqk1nr/pppp1ppp/2n5/2b1p3/2B1P3/5N2/PPPP1PPP/RNBQK2R w KQkq - 4 4
	const pgn = `[Event "Test"]
[Result "*"]
[SetUp "1"]
[FEN "r1bqk1nr/pppp1ppp/2n5/2b1p3/2B1P3/5N2/PPPP1PPP/RNBQK2R w KQkq - 4 4"]

4. b4 *`

	// 1 half-move → 2 analyzePosition calls (N+1).
	//   call 0 (initial, white to move): score=50, best=b2b4  ← b4 is engine best
	//   call 1 (after b4, black to move): score=-55, best=c5b4 ← Black can take
	//
	// For White's b4: scoreBefore=50, scoreAfterFromPlayedSide=55 (negated), delta=+5.
	// isSacrifice=true (Black's bishop on c5 can capture on b4),
	// but sacrificedPieceType=chess.Pawn — pawn sacrifices are excluded from Brilliant.
	// playedMove==bestMove (both b2b4), scoreAfter(55) >= scoreBefore(50),
	// scoreBefore(50) < brilliantWinningThreshold(200) → Best (pawn sacrifice excluded).
	engine := &mockEngine{
		searchInfos: []stockfish.SearchInfo{
			makeDepthInfo(50), makeBestMoveInfo("b2b4"),
			makeDepthInfo(-55), makeBestMoveInfo("c5b4"),
		},
	}

	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	reviews, err := r.ReviewGame(context.Background(), pgn)

	require.NoError(t, err)
	require.Len(t, reviews, 1)

	b4 := reviews[0] // White's b4 is the only move.

	assert.Equal(t, "b2b4", b4.PlayedMove)
	assert.Equal(t, "white", b4.Color)
	assert.True(t, b4.IsSacrifice)
	assert.Equal(t, Best, b4.Classification)
}

// TestReviewer_AnalyzePosition_PrefersExactOverBound verifies that when the
// engine emits a mix of bound scores followed by an exact score, analyzePosition
// returns the exact score rather than the last-seen bound score.
func TestReviewer_AnalyzePosition_PrefersExactOverBound(t *testing.T) {
	t.Parallel()

	const pgn = `[Event "Test"]
[Result "*"]

1. e4 e5 *`

	// analyzePosition is called 3 times (N+1 for 2 moves).
	//
	// call 0 (initial position):
	//   - lowerbound score 80  (should be ignored in favour of exact)
	//   - exact score 20       (this is what we expect back)
	//   - bestmove e2e4
	// call 1 (after e4):  exact 30, bestmove e7e5
	// call 2 (after e5):  exact 10, bestmove d2d4
	//
	// If the exact-preference logic is working, white's ScoreBefore == 20 (not 80)
	// and the delta for white's e4 is -30 - 20 = … wait, scoreAfter = -nextScore.
	// nextScore (call 1) = 30, so scoreAfterFromPlayedSide = -30.
	// delta = -30 - 20 = -50 → Good classification.
	engine := &mockEngine{
		searchInfos: []stockfish.SearchInfo{
			makeBoundInfo(80, stockfish.ScoreBoundLower), // should be overridden
			makeExactInfo(20), // exact — must be preferred
			makeBestMoveInfo("e2e4"),
			makeExactInfo(30), makeBestMoveInfo("e7e5"),
			makeExactInfo(10), makeBestMoveInfo("d2d4"),
		},
	}

	// The mock dispatches batchSize=2 items per call, so we need to override
	// batchSize for this test by using a variable-batch mock. Instead, we embed
	// the three info lines for call 0 into the searchInfos and set batchSize=3
	// for call 0 only.  The standard mockEngine dispatches a fixed batchSize=2,
	// so we use a custom sub-test engine that sends all items at once per call.
	batches := [][]stockfish.SearchInfo{
		{makeBoundInfo(80, stockfish.ScoreBoundLower), makeExactInfo(20), makeBestMoveInfo("e2e4")},
		{makeExactInfo(30), makeBestMoveInfo("e7e5")},
		{makeExactInfo(10), makeBestMoveInfo("d2d4")},
	}

	_ = engine // discard the fixed-batch engine

	batchEngine := &batchMockEngine{batches: batches}
	r := &Reviewer{engine: batchEngine, cfg: defaultConfig()}

	reviews, err := r.ReviewGame(context.Background(), pgn)

	require.NoError(t, err)
	require.Len(t, reviews, 2)

	// White's move: scoreBefore must be the exact value (20), not the lowerbound (80).
	assert.Equal(t, 20, reviews[0].ScoreBefore)
}

// batchMockEngine is a test double that serves pre-defined per-call batches of
// SearchInfo values, allowing variable batch sizes between analyzePosition calls.
type batchMockEngine struct {
	newGameErr error
	batches    [][]stockfish.SearchInfo
	callCount  int
}

func (b *batchMockEngine) NewGame() error { return b.newGameErr }

func (b *batchMockEngine) SetPosition(_ stockfish.Position) error { return nil }

func (b *batchMockEngine) Go(_ context.Context, _ *stockfish.SearchParams) (<-chan stockfish.SearchInfo, error) {
	if b.callCount >= len(b.batches) {
		ch := make(chan stockfish.SearchInfo)
		close(ch)

		return ch, nil
	}

	batch := b.batches[b.callCount]
	b.callCount++

	ch := make(chan stockfish.SearchInfo, len(batch))

	for i := range batch {
		ch <- batch[i]
	}

	close(ch)

	return ch, nil
}

func (b *batchMockEngine) Apply(_ ...stockfish.Option) error { return nil }

func (b *batchMockEngine) Close() error { return nil }

// TestReviewer_ReviewGameFull_PlayerNames verifies that player names parsed from
// PGN White/Black headers are propagated into GameResult.Summary.
func TestReviewer_ReviewGameFull_PlayerNames(t *testing.T) {
	t.Parallel()

	const pgn = `[Event "Test"]
[White "Magnus"]
[Black "Hikaru"]
[Result "*"]

1. e4 e5 *`

	engine := &mockEngine{
		searchInfos: []stockfish.SearchInfo{
			makeDepthInfo(20), makeBestMoveInfo("e2e4"),
			makeDepthInfo(30), makeBestMoveInfo("e7e5"),
			makeDepthInfo(10), makeBestMoveInfo("d2d4"),
		},
	}

	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	result, err := r.ReviewGameFull(context.Background(), pgn)

	require.NoError(t, err)
	assert.Equal(t, "Magnus", result.Summary.WhitePlayer)
	assert.Equal(t, "Hikaru", result.Summary.BlackPlayer)
}

// TestReviewer_ReviewGameFull_ReviewsMatchReviewGame confirms that the Reviews
// slice returned by ReviewGameFull is identical to what ReviewGame returns for
// the same PGN and engine responses.
func TestReviewer_ReviewGameFull_ReviewsMatchReviewGame(t *testing.T) {
	t.Parallel()

	const pgn = `[Event "Test"]
[White "White"]
[Black "Black"]
[Result "*"]

1. e4 e5 *`

	makeInfos := func() []stockfish.SearchInfo {
		return []stockfish.SearchInfo{
			makeDepthInfo(20), makeBestMoveInfo("e2e4"),
			makeDepthInfo(30), makeBestMoveInfo("e7e5"),
			makeDepthInfo(10), makeBestMoveInfo("d2d4"),
		}
	}

	engineFull := &mockEngine{searchInfos: makeInfos()}
	engineGame := &mockEngine{searchInfos: makeInfos()}

	rFull := &Reviewer{engine: engineFull, cfg: defaultConfig()}
	rGame := &Reviewer{engine: engineGame, cfg: defaultConfig()}

	result, err := rFull.ReviewGameFull(context.Background(), pgn)
	require.NoError(t, err)

	reviews, err := rGame.ReviewGame(context.Background(), pgn)
	require.NoError(t, err)

	require.Equal(t, reviews, result.Reviews)
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

// TestReviewer_ReviewGame_BookMoves verifies that moves recognised as opening
// theory are classified as Book in the review output, regardless of the engine
// score, and that IsBook is propagated onto the MoveReview.
func TestReviewer_ReviewGame_BookMoves(t *testing.T) {
	t.Parallel()

	// Italian Game opening — all five moves are ECO book moves.
	const pgn = `[Event "Test"]
[Result "*"]

1. e4 e5 2. Nf3 Nc6 3. Bc4 *`

	// 5 moves → 6 analyzePosition calls. Engine scores don't matter for Book
	// classification, but we still need valid responses for the loop to complete.
	engine := &mockEngine{
		searchInfos: []stockfish.SearchInfo{
			makeDepthInfo(20), makeBestMoveInfo("e2e4"),
			makeDepthInfo(30), makeBestMoveInfo("e7e5"),
			makeDepthInfo(20), makeBestMoveInfo("g1f3"),
			makeDepthInfo(25), makeBestMoveInfo("b8c6"),
			makeDepthInfo(30), makeBestMoveInfo("f1c4"),
			makeDepthInfo(25), makeBestMoveInfo("d7d6"),
		},
	}

	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	reviews, err := r.ReviewGame(context.Background(), pgn)

	require.NoError(t, err)
	require.Len(t, reviews, 5)

	for i, rv := range reviews {
		assert.True(t, rv.IsBook, "expected move %d (%s) to have IsBook=true", i, rv.PlayedMove)
		assert.Equal(t, Book, rv.Classification, "expected move %d (%s) to be classified as Book", i, rv.PlayedMove)
	}
}

// TestReviewer_ReviewGameFull_OpeningInSummary verifies that the ECO opening
// code and title detected from the moves are propagated into the GameSummary.
func TestReviewer_ReviewGameFull_OpeningInSummary(t *testing.T) {
	t.Parallel()

	// Italian Game opening.
	const pgn = `[Event "Test"]
[White "White"]
[Black "Black"]
[Result "*"]

1. e4 e5 2. Nf3 Nc6 3. Bc4 *`

	engine := &mockEngine{
		searchInfos: []stockfish.SearchInfo{
			makeDepthInfo(20), makeBestMoveInfo("e2e4"),
			makeDepthInfo(30), makeBestMoveInfo("e7e5"),
			makeDepthInfo(20), makeBestMoveInfo("g1f3"),
			makeDepthInfo(25), makeBestMoveInfo("b8c6"),
			makeDepthInfo(30), makeBestMoveInfo("f1c4"),
			makeDepthInfo(25), makeBestMoveInfo("d7d6"),
		},
	}

	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	result, err := r.ReviewGameFull(context.Background(), pgn)

	require.NoError(t, err)
	assert.Equal(t, "C50", result.Summary.OpeningCode, "expected ECO code C50 in summary")
	assert.Equal(t, "Italian Game", result.Summary.OpeningTitle, "expected opening title 'Italian Game' in summary")
}

// TestReviewer_ReviewGameStream_TwoMoves verifies that ReviewGameStream emits
// each MoveReview as soon as it is computed and that the moves channel is
// closed with no error after all moves are processed.
func TestReviewer_ReviewGameStream_TwoMoves(t *testing.T) {
	t.Parallel()

	const pgn = `[Event "Test"]
[Site "?"]
[Date "????.??.??"]
[Round "?"]
[White "White"]
[Black "Black"]
[Result "*"]

1. e4 e5 *`

	engine := &mockEngine{
		searchInfos: []stockfish.SearchInfo{
			makeDepthInfo(20), makeBestMoveInfo("e2e4"),
			makeDepthInfo(30), makeBestMoveInfo("e7e5"),
			makeDepthInfo(10), makeBestMoveInfo("d2d4"),
		},
	}

	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	movesCh, errCh := r.ReviewGameStream(context.Background(), pgn)

	var reviews []MoveReview

	for mr := range movesCh {
		reviews = append(reviews, mr)
	}

	// Error channel must be closed with no error.
	err, ok := <-errCh
	assert.False(t, ok, "error channel should be closed")
	assert.NoError(t, err)

	require.Len(t, reviews, 2)

	assert.Equal(t, "e2e4", reviews[0].PlayedMove)
	assert.Equal(t, "white", reviews[0].Color)
	assert.Equal(t, Book, reviews[0].Classification)

	assert.Equal(t, "e7e5", reviews[1].PlayedMove)
	assert.Equal(t, "black", reviews[1].Color)
	assert.Equal(t, Book, reviews[1].Classification)
}

// TestReviewer_ReviewGameStream_InvalidPGN verifies that ReviewGameStream
// sends an ErrInvalidPGN on the error channel and closes the moves channel
// immediately when given invalid PGN.
func TestReviewer_ReviewGameStream_InvalidPGN(t *testing.T) {
	t.Parallel()

	engine := &mockEngine{}
	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	movesCh, errCh := r.ReviewGameStream(context.Background(), "not valid pgn!!!")

	// Moves channel should be closed immediately.
	_, ok := <-movesCh
	assert.False(t, ok, "moves channel should be closed on parse error")

	err, ok := <-errCh
	require.True(t, ok, "error channel should carry the parse error")
	require.Error(t, err)

	var pgnErr *ErrInvalidPGN

	assert.ErrorAs(t, err, &pgnErr)
}

// TestReviewer_ReviewGameStream_EngineError verifies that ReviewGameStream
// sends an ErrEngineFailure on the error channel when the engine fails during
// analysis.
func TestReviewer_ReviewGameStream_EngineError(t *testing.T) {
	t.Parallel()

	const pgn = `[Event "Test"]
[Result "*"]

1. e4 e5 *`

	engine := &mockEngine{goErr: errors.New("engine exploded")}
	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	movesCh, errCh := r.ReviewGameStream(context.Background(), pgn)

	// Drain the moves channel.
	for range movesCh {
	}

	err, ok := <-errCh
	require.True(t, ok, "error channel should carry the engine error")
	require.Error(t, err)

	var engErr *ErrEngineFailure

	assert.ErrorAs(t, err, &engErr)
}

// TestReviewer_ReviewGameStream_ZeroValue verifies that ReviewGameStream on an
// uninitialised Reviewer sends ErrEngineFailure and closes channels immediately.
func TestReviewer_ReviewGameStream_ZeroValue(t *testing.T) {
	t.Parallel()

	var r Reviewer

	movesCh, errCh := r.ReviewGameStream(context.Background(), "1. e4 e5 *")

	_, ok := <-movesCh
	assert.False(t, ok, "moves channel should be closed")

	err, ok := <-errCh
	require.True(t, ok)
	require.Error(t, err)

	var engErr *ErrEngineFailure

	assert.ErrorAs(t, err, &engErr)
	assert.Contains(t, engErr.Error(), "not initialized")
}

// TestReviewer_ReviewGameFullStream_TwoMoves verifies that ReviewGameFullStream
// streams move reviews, emits a GameSummary after all moves, and closes all
// channels cleanly.
func TestReviewer_ReviewGameFullStream_TwoMoves(t *testing.T) {
	t.Parallel()

	const pgn = `[Event "Test"]
[White "Magnus"]
[Black "Hikaru"]
[Result "*"]

1. e4 e5 *`

	engine := &mockEngine{
		searchInfos: []stockfish.SearchInfo{
			makeDepthInfo(20), makeBestMoveInfo("e2e4"),
			makeDepthInfo(30), makeBestMoveInfo("e7e5"),
			makeDepthInfo(10), makeBestMoveInfo("d2d4"),
		},
	}

	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	movesCh, errCh, summariesCh := r.ReviewGameFullStream(context.Background(), pgn)

	var reviews []MoveReview

	for mr := range movesCh {
		reviews = append(reviews, mr)
	}

	// Error channel must be closed with no error.
	err, ok := <-errCh
	assert.False(t, ok, "error channel should be closed")
	assert.NoError(t, err)

	// Summary channel must carry exactly one value.
	summary, ok := <-summariesCh
	require.True(t, ok, "summary channel should carry a GameSummary")

	_, closed := <-summariesCh
	assert.False(t, closed, "summary channel should be closed after the summary")

	require.Len(t, reviews, 2)
	assert.Equal(t, "Magnus", summary.WhitePlayer)
	assert.Equal(t, "Hikaru", summary.BlackPlayer)
}

// TestReviewer_ReviewGameFullStream_ReviewsMatchReviewGame verifies that the
// reviews streamed by ReviewGameFullStream are identical to those returned by
// ReviewGame for the same PGN and engine responses.
func TestReviewer_ReviewGameFullStream_ReviewsMatchReviewGame(t *testing.T) {
	t.Parallel()

	const pgn = `[Event "Test"]
[White "White"]
[Black "Black"]
[Result "*"]

1. e4 e5 *`

	makeInfos := func() []stockfish.SearchInfo {
		return []stockfish.SearchInfo{
			makeDepthInfo(20), makeBestMoveInfo("e2e4"),
			makeDepthInfo(30), makeBestMoveInfo("e7e5"),
			makeDepthInfo(10), makeBestMoveInfo("d2d4"),
		}
	}

	engineStream := &mockEngine{searchInfos: makeInfos()}
	engineGame := &mockEngine{searchInfos: makeInfos()}

	rStream := &Reviewer{engine: engineStream, cfg: defaultConfig()}
	rGame := &Reviewer{engine: engineGame, cfg: defaultConfig()}

	movesCh, _, _ := rStream.ReviewGameFullStream(context.Background(), pgn)

	var streamedReviews []MoveReview

	for mr := range movesCh {
		streamedReviews = append(streamedReviews, mr)
	}

	gameReviews, err := rGame.ReviewGame(context.Background(), pgn)
	require.NoError(t, err)

	require.Equal(t, gameReviews, streamedReviews)
}

// TestReviewer_ReviewGameFullStream_InvalidPGN verifies that
// ReviewGameFullStream sends ErrInvalidPGN on the error channel and closes all
// channels without sending a summary.
func TestReviewer_ReviewGameFullStream_InvalidPGN(t *testing.T) {
	t.Parallel()

	engine := &mockEngine{}
	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	movesCh, errCh, summariesCh := r.ReviewGameFullStream(context.Background(), "not valid pgn!!!")

	_, ok := <-movesCh
	assert.False(t, ok, "moves channel should be closed on parse error")

	err, ok := <-errCh
	require.True(t, ok, "error channel should carry the parse error")

	var pgnErr *ErrInvalidPGN

	assert.ErrorAs(t, err, &pgnErr)

	_, ok = <-summariesCh
	assert.False(t, ok, "summary channel should be closed without a value on error")
}

// TestReviewer_ReviewGameFullStream_EngineError verifies that
// ReviewGameFullStream sends ErrEngineFailure on the error channel and closes
// the summary channel without a value when the engine fails.
func TestReviewer_ReviewGameFullStream_EngineError(t *testing.T) {
	t.Parallel()

	const pgn = `[Event "Test"]
[Result "*"]

1. e4 e5 *`

	engine := &mockEngine{goErr: errors.New("engine exploded")}
	r := &Reviewer{engine: engine, cfg: defaultConfig()}

	movesCh, errCh, summariesCh := r.ReviewGameFullStream(context.Background(), pgn)

	for range movesCh {
	}

	err, ok := <-errCh
	require.True(t, ok, "error channel should carry the engine error")

	var engErr *ErrEngineFailure

	assert.ErrorAs(t, err, &engErr)

	_, ok = <-summariesCh
	assert.False(t, ok, "summary channel should be closed without a value on error")
}

// TestReviewer_ReviewGameFullStream_ZeroValue verifies that
// ReviewGameFullStream on an uninitialised Reviewer sends ErrEngineFailure and
// closes all channels immediately.
func TestReviewer_ReviewGameFullStream_ZeroValue(t *testing.T) {
	t.Parallel()

	var r Reviewer

	movesCh, errCh, summariesCh := r.ReviewGameFullStream(context.Background(), "1. e4 e5 *")

	_, ok := <-movesCh
	assert.False(t, ok, "moves channel should be closed")

	err, ok := <-errCh
	require.True(t, ok)
	require.Error(t, err)

	var engErr *ErrEngineFailure

	assert.ErrorAs(t, err, &engErr)
	assert.Contains(t, engErr.Error(), "not initialized")

	_, ok = <-summariesCh
	assert.False(t, ok, "summary channel should be closed")
}

// TestReviewer_ReviewGame_MultiPV verifies that TopMoves on each MoveReview
// contains all candidate moves returned by the engine when MultiPV > 1, ordered
// by PV index from best to worst, with correct scores.
func TestReviewer_ReviewGame_MultiPV(t *testing.T) {
	t.Parallel()

	// Use a SetUp FEN with the standard starting position.
	// Note: detectOpenings will still run (the FEN matches the standard start
	// prefix), so 1.d4 may be classified as a book move. This test focuses on
	// MultiPV evaluation (TopMoves ordering and scores), not on book detection.
	const pgn = `[Event "Test"]
[Result "*"]
[SetUp "1"]
[FEN "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"]

1. d4 *`

	// 1 half-move → 2 analyzePosition calls.
	//   call 0 (initial, white to move): 3 PVs
	//     PV1: d2d4, score=30
	//     PV2: e2e4, score=20
	//     PV3: c2c4, score=15
	//   call 1 (after d4, black to move): 3 PVs — only used to drive next iteration.
	//     PV1: d7d5, score=25
	//     PV2: g8f6, score=20
	//     PV3: e7e6, score=18
	batches := [][]stockfish.SearchInfo{
		{
			makeMultiPVInfo(1, 30, "d2d4"),
			makeMultiPVInfo(2, 20, "e2e4"),
			makeMultiPVInfo(3, 15, "c2c4"),
			makeBestMoveInfo("d2d4"),
		},
		{
			makeMultiPVInfo(1, 25, "d7d5"),
			makeMultiPVInfo(2, 20, "g8f6"),
			makeMultiPVInfo(3, 18, "e7e6"),
			makeBestMoveInfo("d7d5"),
		},
	}

	r := &Reviewer{engine: &batchMockEngine{batches: batches}, cfg: defaultConfig()}

	reviews, err := r.ReviewGame(context.Background(), pgn)

	require.NoError(t, err)
	require.Len(t, reviews, 1)

	top := reviews[0].TopMoves
	require.Len(t, top, 3, "expected 3 top moves from MultiPV")

	assert.Equal(t, "d2d4", top[0].Move)
	assert.Equal(t, 30, top[0].Score)
	assert.Nil(t, top[0].MateIn)

	assert.Equal(t, "e2e4", top[1].Move)
	assert.Equal(t, 20, top[1].Score)

	assert.Equal(t, "c2c4", top[2].Move)
	assert.Equal(t, 15, top[2].Score)
}

// TestReviewer_ReviewGame_CheckmateTerminal verifies that ReviewGame succeeds
// when the final move delivers checkmate. Stockfish responds with
// "bestmove (none)" for the resulting position; reviewFromGameInfo must
// synthesise a terminal evaluation rather than calling analyzePosition and
// returning ErrEngineFailure.
func TestReviewer_ReviewGame_CheckmateTerminal(t *testing.T) {
	t.Parallel()

	// Ra8# — rook to a8 delivers checkmate.
	// FEN: 5k2/R7/5K2/8/8/8/8/8 w - - 0 1
	// After Ra8 (a7a8) the black king on f8 is checkmated.
	const pgn = `[Event "Test"]
[Result "1-0"]
[SetUp "1"]
[FEN "5k2/R7/5K2/8/8/8/8/8 w - - 0 1"]

1. Ra8# 1-0`

	// analyzePosition is called only once (N+1 = 1+1, but the terminal call is
	// skipped). Total engine calls: 1 (initial position before Ra8).
	//
	// call 0 (initial, white to move): mate-in-1 available; best = a7a8.
	//
	// The post-move position is terminal (checkmate), so reviewFromGameInfo
	// synthesises nextTopMoves without calling analyzePosition again.
	batches := [][]stockfish.SearchInfo{
		{makeMateInfo(1), makeBestMoveInfo("a7a8")},
	}

	r := &Reviewer{engine: &batchMockEngine{batches: batches}, cfg: defaultConfig()}

	reviews, err := r.ReviewGame(context.Background(), pgn)

	require.NoError(t, err)
	require.Len(t, reviews, 1)

	rv := reviews[0]

	assert.Equal(t, "a7a8", rv.PlayedMove)
	assert.Equal(t, "white", rv.Color)

	// ScoreBefore: the engine reported mate-in-1 before the move.
	assert.Equal(t, mateScoreSentinel, rv.ScoreBefore)
	require.NotNil(t, rv.MateInBefore)
	assert.Equal(t, 1, *rv.MateInBefore)

	// ScoreAfter: synthesised as -(-mateScoreSentinel) = +mateScoreSentinel from
	// white's (played side's) perspective — white delivered checkmate.
	assert.Equal(t, mateScoreSentinel, rv.ScoreAfter)

	// MateInAfter: synthesised mate-in-0, negated into played side's frame → 0.
	require.NotNil(t, rv.MateInAfter)
	assert.Equal(t, 0, *rv.MateInAfter)
}

// TestReviewer_ReviewGame_StalemateTerminal verifies that ReviewGame succeeds
// when the final move produces stalemate. The synthesised post-move evaluation
// must carry Score=0 (a draw), not -mateScoreSentinel.
func TestReviewer_ReviewGame_StalemateTerminal(t *testing.T) {
	t.Parallel()

	// Qb6 stalemate: white queen moves from c7 to b6, leaving the black king on
	// a8 with no legal moves and not in check.
	// FEN: k7/2Q5/2K5/8/8/8/8/8 w - - 0 1
	// After Qc7-b6 (c7b6) the black king on a8 is stalemated.
	const pgn = `[Event "Test"]
[Result "1/2-1/2"]
[SetUp "1"]
[FEN "k7/2Q5/2K5/8/8/8/8/8 w - - 0 1"]

1. Qb6 1/2-1/2`

	// call 0 (initial, white to move): cp score 500, best = c7b6.
	batches := [][]stockfish.SearchInfo{
		{makeDepthInfo(500), makeBestMoveInfo("c7b6")},
	}

	r := &Reviewer{engine: &batchMockEngine{batches: batches}, cfg: defaultConfig()}

	reviews, err := r.ReviewGame(context.Background(), pgn)

	require.NoError(t, err)
	require.Len(t, reviews, 1)

	rv := reviews[0]

	assert.Equal(t, "c7b6", rv.PlayedMove)
	assert.Equal(t, "white", rv.Color)

	// ScoreAfter: synthesised as -(0) = 0 (draw) from the played side's frame.
	assert.Equal(t, 0, rv.ScoreAfter)

	// MateInAfter: stalemate is a draw, so it should not be represented as mate-in-0.
	assert.Nil(t, rv.MateInAfter)
}

// TestAnalyzePosition_SafetyNet_BestMoveNone verifies the safety-net guard in
// analyzePosition: when the engine sends only a bestmove=(none) line (no info
// lines), the function must return a synthetic mate-0 evaluation rather than
// ErrEngineFailure. This covers direct calls to analyzePosition on terminal
// positions that bypass the reviewFromGameInfo short-circuit.
func TestAnalyzePosition_SafetyNet_BestMoveNone(t *testing.T) {
	t.Parallel()

	// Engine sends only the bestmove (none) line — no info lines at all.
	batches := [][]stockfish.SearchInfo{
		{makeBestMoveInfo("(none)")},
	}

	r := &Reviewer{engine: &batchMockEngine{batches: batches}, cfg: defaultConfig()}

	initialFEN := "5k2/R7/5K2/8/8/8/8/8 w - - 0 1"
	evals, err := r.analyzePosition(context.Background(), initialFEN, []string{"a7a8"})

	require.NoError(t, err)
	require.Len(t, evals, 1)

	// Score must be -mateScoreSentinel (the side to move is already mated).
	assert.Equal(t, -mateScoreSentinel, evals[0].Score)

	// MateIn must be 0 (already mated).
	require.NotNil(t, evals[0].MateIn)
	assert.Equal(t, 0, *evals[0].MateIn)
}
