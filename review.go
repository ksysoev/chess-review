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

	client, err := stockfish.New(stockfishPath)
	if err != nil {
		return nil, &ErrEngineFailure{Reason: fmt.Sprintf("failed to start engine: %s", err.Error())}
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
	return r.engine.Close()
}

// ReviewGame analyses the provided PGN string and returns a MoveReview for each
// half-move in the game. It evaluates both the position before and after each
// played move at the configured depth to determine the centipawn loss.
//
// Returns ErrInvalidPGN when the PGN cannot be parsed.
// Returns ErrEngineFailure when communication with the engine fails.
func (r *Reviewer) ReviewGame(ctx context.Context, pgn string) ([]MoveReview, error) {
	moves, err := parsePGN(pgn)
	if err != nil {
		return nil, err
	}

	if err = r.engine.NewGame(); err != nil {
		return nil, &ErrEngineFailure{Reason: fmt.Sprintf("ucinewgame failed: %s", err.Error())}
	}

	reviews := make([]MoveReview, 0, len(moves))
	playedSoFar := make([]string, 0, len(moves))

	for _, mv := range moves {
		scoreBefore, bestMove, analyzeErr := r.analyzePosition(ctx, playedSoFar)
		if analyzeErr != nil {
			return nil, analyzeErr
		}

		playedSoFar = append(playedSoFar, mv.UCIMove)

		scoreAfter, _, analyzeErr := r.analyzePosition(ctx, playedSoFar)
		if analyzeErr != nil {
			return nil, analyzeErr
		}

		// Negate scoreAfter: after the move, Stockfish evaluates from the opponent's
		// perspective, so we flip the sign to keep it in the played-side's frame.
		scoreAfterFromPlayedSide := -scoreAfter
		delta := scoreAfterFromPlayedSide - scoreBefore

		reviews = append(reviews, MoveReview{
			PlayedMove:     mv.UCIMove,
			BestMove:       bestMove,
			Color:          mv.Color,
			MoveNumber:     mv.MoveNumber,
			ScoreBefore:    scoreBefore,
			ScoreAfter:     scoreAfterFromPlayedSide,
			ScoreDelta:     delta,
			Classification: Classify(delta, mv.UCIMove, bestMove),
		})
	}

	return reviews, nil
}

// analyzePosition sets the engine position to the given sequence of UCI moves
// starting from the initial position, runs a depth-limited search, and returns
// the centipawn score and best move.
func (r *Reviewer) analyzePosition(ctx context.Context, moves []string) (score int, bestMove string, err error) {
	pos := stockfish.StartPosition()
	if len(moves) > 0 {
		pos = pos.WithMoves(moves...)
	}

	if setErr := r.engine.SetPosition(pos); setErr != nil {
		return 0, "", &ErrEngineFailure{Reason: fmt.Sprintf("set position failed: %s", setErr.Error())}
	}

	ch, goErr := r.engine.Go(ctx, &stockfish.SearchParams{Depth: r.cfg.depth})
	if goErr != nil {
		return 0, "", &ErrEngineFailure{Reason: fmt.Sprintf("go command failed: %s", goErr.Error())}
	}

	for info := range ch {
		if info.IsBestMove {
			bestMove = info.BestMove

			break
		}

		score = info.Score.Value
	}

	return score, bestMove, nil
}

// applyEngineOptions configures the engine with the settings from cfg.
func (r *Reviewer) applyEngineOptions() error {
	err := r.engine.Apply(
		stockfish.WithThreads(r.cfg.threads),
		stockfish.WithHash(r.cfg.hashMB),
	)
	if err != nil {
		return &ErrEngineFailure{Reason: fmt.Sprintf("failed to apply engine options: %s", err.Error())}
	}

	return nil
}
