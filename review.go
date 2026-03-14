package chessreview

import (
	"context"
	"fmt"

	"github.com/ksysoev/stockfish"
)

// chessEngine is the interface used to interact with the Stockfish engine.
// It is defined here so it can be mocked in tests.
type chessEngine interface {
	NewGame() error
	SetPosition(pos stockfish.Position) error
	Go(ctx context.Context, params *stockfish.SearchParams) (<-chan stockfish.SearchInfo, error)
	Apply(opts ...stockfish.Option) error
	Close() error
}

// MoveReview holds the analysis result for a single half-move (ply).
type MoveReview struct {
	// PlayedMove is the move that was actually played, in UCI notation (e.g. "e2e4").
	PlayedMove string
	// BestMove is the engine's top-recommended move at the given depth.
	BestMove string
	// Color is the side that played: "white" or "black".
	Color string
	// Classification is the quality rating of the played move.
	Classification Classification
	// ScoreBefore is the centipawn evaluation before the move, from the perspective
	// of the side to move.
	ScoreBefore int
	// ScoreAfter is the centipawn evaluation after the move, from the perspective
	// of the side that just moved (negated to match same reference frame).
	ScoreAfter int
	// ScoreDelta is the change in centipawns (ScoreAfter - ScoreBefore).
	// Negative values indicate centipawn loss.
	ScoreDelta int
	// MoveNumber is the full-move number (1-indexed; increments after Black's move).
	MoveNumber int
}

// Reviewer analyses chess games using a Stockfish engine.
// It must be created with New; the zero value is not usable.
type Reviewer struct {
	engine chessEngine
	cfg    config
}

// New creates a new Reviewer that uses the Stockfish binary at stockfishPath.
// Optional functional options can be provided to configure depth, threads, and
// hash table size.
//
// Returns ErrEngineFailure if the engine cannot be started.
func New(stockfishPath string, opts ...Option) (*Reviewer, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	if err := cfg.validate(); err != nil {
		return nil, &ErrEngineFailure{Cause: err, Reason: err.Error()}
	}

	client, err := stockfish.New(stockfishPath)
	if err != nil {
		return nil, &ErrEngineFailure{Cause: err, Reason: fmt.Sprintf("failed to start engine: %s", err.Error())}
	}

	r := &Reviewer{
		engine: client,
		cfg:    cfg,
	}

	if err = r.applyEngineOptions(); err != nil {
		_ = client.Close()

		return nil, err
	}

	return r, nil
}

// Close shuts down the underlying Stockfish engine process.
func (r *Reviewer) Close() error {
	if r.engine == nil {
		return &ErrEngineFailure{Reason: "reviewer not initialized; use New()"}
	}

	return r.engine.Close()
}

// ReviewGame analyses the provided PGN string and returns a MoveReview for each
// half-move in the game. It evaluates each position once at the configured depth
// (N+1 total calls for N plies) and carries the score forward between plies to
// avoid redundant engine work.
//
// PGN games with a custom starting position (SetUp/FEN headers) are fully
// supported: Stockfish is initialised from the FEN rather than the standard
// starting position, and move numbers reflect the FEN's full-move counter.
//
// Returns ErrInvalidPGN when the PGN cannot be parsed.
// Returns ErrEngineFailure when communication with the engine fails.
func (r *Reviewer) ReviewGame(ctx context.Context, pgn string) ([]MoveReview, error) {
	if r.engine == nil {
		return nil, &ErrEngineFailure{Reason: "reviewer not initialized; use New()"}
	}

	gi, err := parsePGN(pgn)
	if err != nil {
		return nil, err
	}

	if err = r.engine.NewGame(); err != nil {
		return nil, &ErrEngineFailure{Cause: err, Reason: fmt.Sprintf("ucinewgame failed: %s", err.Error())}
	}

	reviews := make([]MoveReview, 0, len(gi.Moves))
	playedSoFar := make([]string, 0, len(gi.Moves))

	// Evaluate the initial position once before the loop.
	currentScore, bestMove, analyzeErr := r.analyzePosition(ctx, gi.InitialFEN, playedSoFar)
	if analyzeErr != nil {
		return nil, analyzeErr
	}

	for _, mv := range gi.Moves {
		scoreBefore := currentScore
		thisBestMove := bestMove

		playedSoFar = append(playedSoFar, mv.UCIMove)

		nextScore, nextBestMove, analyzeErr := r.analyzePosition(ctx, gi.InitialFEN, playedSoFar)
		if analyzeErr != nil {
			return nil, analyzeErr
		}

		// Negate nextScore: after the move Stockfish evaluates from the opponent's
		// perspective, so we flip the sign to keep it in the played-side's frame.
		scoreAfterFromPlayedSide := -nextScore
		delta := scoreAfterFromPlayedSide - scoreBefore

		reviews = append(reviews, MoveReview{
			PlayedMove:     mv.UCIMove,
			BestMove:       thisBestMove,
			Color:          mv.Color,
			MoveNumber:     mv.MoveNumber,
			ScoreBefore:    scoreBefore,
			ScoreAfter:     scoreAfterFromPlayedSide,
			ScoreDelta:     delta,
			Classification: Classify(delta, mv.UCIMove, thisBestMove),
		})

		// Carry forward: the opponent's next "before" score is -nextScore from
		// their perspective (already done via the negation above), but for the
		// next iteration currentScore must be in the next side-to-move's frame,
		// which is exactly nextScore as returned by the engine.
		currentScore = nextScore
		bestMove = nextBestMove
	}

	return reviews, nil
}

// mateScoreSentinel is the centipawn value used to represent a forced mate.
// Positive means the side to move has a forced mate; negative means they are
// being mated. The magnitude is chosen to be far outside any real centipawn
// range while still leaving room for delta arithmetic without overflow.
const mateScoreSentinel = 30_000

// analyzePosition sets the engine position to the given sequence of UCI moves
// starting from initialFEN, runs a depth-limited search, and returns the
// centipawn score and best move.
//
// initialFEN must be a valid FEN string (typically the first position of the
// parsed game). Using the game's actual starting FEN rather than always
// stockfish.StartPosition ensures correctness for PGNs with SetUp/FEN headers.
//
// Mate scores reported by the engine are mapped to ±mateScoreSentinel so that
// downstream centipawn arithmetic remains meaningful. Returns ErrEngineFailure
// if the engine stream closes without ever producing a best move.
func (r *Reviewer) analyzePosition(ctx context.Context, initialFEN string, moves []string) (score int, bestMove string, err error) {
	pos := stockfish.FENPosition(initialFEN)
	if len(moves) > 0 {
		pos = pos.WithMoves(moves...)
	}

	if setErr := r.engine.SetPosition(pos); setErr != nil {
		return 0, "", &ErrEngineFailure{Cause: setErr, Reason: fmt.Sprintf("set position failed: %s", setErr.Error())}
	}

	ch, goErr := r.engine.Go(ctx, &stockfish.SearchParams{Depth: r.cfg.depth})
	if goErr != nil {
		return 0, "", &ErrEngineFailure{Cause: goErr, Reason: fmt.Sprintf("go command failed: %s", goErr.Error())}
	}

	bestMoveFound := false

	for info := range ch {
		if info.IsBestMove {
			bestMove = info.BestMove
			bestMoveFound = true

			break
		}

		score = normalizeScore(info.Score)
	}

	if !bestMoveFound {
		return 0, "", &ErrEngineFailure{Reason: "engine returned no best move"}
	}

	return score, bestMove, nil
}

// normalizeScore converts a stockfish Score to a centipawn integer.
// Centipawn scores are returned as-is. Mate scores are mapped to
// ±mateScoreSentinel: positive when the side to move has a forced mate,
// negative when they are being mated.
func normalizeScore(s stockfish.Score) int {
	if s.Type != stockfish.ScoreTypeMate {
		return s.Value
	}

	if s.Value >= 0 {
		return mateScoreSentinel
	}

	return -mateScoreSentinel
}

// applyEngineOptions configures the engine with the settings from cfg.
func (r *Reviewer) applyEngineOptions() error {
	err := r.engine.Apply(
		stockfish.WithThreads(r.cfg.threads),
		stockfish.WithHash(r.cfg.hashMB),
	)
	if err != nil {
		return &ErrEngineFailure{Cause: err, Reason: fmt.Sprintf("failed to apply engine options: %s", err.Error())}
	}

	return nil
}
