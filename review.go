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
	// MateInBefore is non-nil when the position before this move has a forced-mate
	// sequence. Positive means the side to move can force checkmate in that many
	// moves; negative means they are being mated in that many moves.
	MateInBefore *int
	// MateInAfter is non-nil when the position after this move has a forced-mate
	// sequence, expressed from the perspective of the side that just moved.
	// Positive means the side that just moved can still force checkmate in that many
	// moves; negative means the opponent now has the forced mate in that many moves.
	MateInAfter *int
	// PlayedMove is the move that was actually played, in UCI notation (e.g. "e2e4").
	PlayedMove string
	// BestMove is the engine's top-recommended move at the given depth.
	BestMove string
	// Color is the side that played: "white" or "black".
	Color string
	// Classification is the quality rating of the played move.
	Classification Classification
	// IsSacrifice is true when the move was detected as a material sacrifice:
	// the moved piece's value exceeds what was captured, and the opponent had
	// at least one legal recapture on the destination square.
	IsSacrifice bool
	// IsBook is true when the move is part of a known ECO opening line.
	// Book moves take priority over engine-based classifications and are
	// excluded from accuracy calculations.
	IsBook bool
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

// GameResult holds the full output of a game review: per-move analysis and an
// aggregated summary for both players.
type GameResult struct {
	Reviews []MoveReview
	Summary GameSummary
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

	return r.reviewFromGameInfo(ctx, &gi, nil)
}

// reviewFromGameInfo runs the engine analysis loop over an already-parsed game.
// It is shared by ReviewGame, ReviewGameFull, ReviewGameStream, and
// ReviewGameFullStream to avoid duplication.
//
// Every MoveReview is always collected into the returned slice. When sink is
// non-nil each review is also sent to it as soon as it is computed; the send
// is wrapped in a select on ctx.Done() so that context cancellation unblocks
// the producer and returns ctx.Err() instead of blocking indefinitely.
func (r *Reviewer) reviewFromGameInfo(ctx context.Context, gi *gameInfo, sink chan<- MoveReview) ([]MoveReview, error) {
	if err := r.engine.NewGame(); err != nil {
		return nil, &ErrEngineFailure{Cause: err, Reason: fmt.Sprintf("ucinewgame failed: %s", err.Error())}
	}

	reviews := make([]MoveReview, 0, len(gi.Moves))

	playedSoFar := make([]string, 0, len(gi.Moves))

	// Evaluate the initial position once before the loop.
	currentScore, bestMove, currentMateIn, analyzeErr := r.analyzePosition(ctx, gi.InitialFEN, playedSoFar)
	if analyzeErr != nil {
		return nil, analyzeErr
	}

	// prevScoreBefore tracks each colour's own ScoreBefore from their previous
	// turn. This enables the 2-ply lookback in Classify: when an opponent
	// blunders, the player who capitalises may deserve a Great annotation even
	// though their position was already winning at the start of their turn.
	prevScoreBefore := map[string]int{}
	hasPrev := map[string]bool{}

	for _, mv := range gi.Moves {
		scoreBefore := currentScore
		thisBestMove := bestMove
		mateInBefore := currentMateIn

		playedSoFar = append(playedSoFar, mv.UCIMove)

		nextScore, nextBestMove, nextMateIn, analyzeErr := r.analyzePosition(ctx, gi.InitialFEN, playedSoFar)
		if analyzeErr != nil {
			return nil, analyzeErr
		}

		// Negate nextScore: after the move Stockfish evaluates from the opponent's
		// perspective, so we flip the sign to keep it in the played-side's frame.
		scoreAfterFromPlayedSide := -nextScore
		delta := scoreAfterFromPlayedSide - scoreBefore

		// Flip nextMateIn into the played side's frame: if the opponent now has
		// mate-in-N (positive from their POV), that is -N from the played side.
		var mateInAfter *int

		if nextMateIn != nil {
			v := -*nextMateIn
			mateInAfter = &v
		}

		classCtx := ClassifyContext{
			PlayedMove:      mv.UCIMove,
			BestMove:        thisBestMove,
			ScoreBefore:     scoreBefore,
			ScoreAfter:      scoreAfterFromPlayedSide,
			ScoreBeforePrev: prevScoreBefore[mv.Color],
			HasPrev:         hasPrev[mv.Color],
			IsSacrifice:     mv.IsSacrifice,
			IsBook:          mv.IsBook,
		}

		mr := MoveReview{
			PlayedMove:     mv.UCIMove,
			BestMove:       thisBestMove,
			Color:          mv.Color,
			MoveNumber:     mv.MoveNumber,
			ScoreBefore:    scoreBefore,
			ScoreAfter:     scoreAfterFromPlayedSide,
			ScoreDelta:     delta,
			Classification: Classify(classCtx),
			IsSacrifice:    mv.IsSacrifice,
			IsBook:         mv.IsBook,
			MateInBefore:   mateInBefore,
			MateInAfter:    mateInAfter,
		}

		reviews = append(reviews, mr)

		if sink != nil {
			select {
			case sink <- mr:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// Update the per-colour lookback state for the next turn of this colour.
		prevScoreBefore[mv.Color] = scoreBefore
		hasPrev[mv.Color] = true

		// Carry forward: the opponent's next "before" score is -nextScore from
		// their perspective (already done via the negation above), but for the
		// next iteration currentScore must be in the next side-to-move's frame,
		// which is exactly nextScore as returned by the engine.
		currentScore = nextScore
		bestMove = nextBestMove
		currentMateIn = nextMateIn
	}

	return reviews, nil
}

// analyzePosition sets the engine position to the given sequence of UCI moves
// starting from initialFEN, runs a depth-limited search, and returns the
// centipawn score, best move, and forced-mate distance (if any).
//
// initialFEN must be a valid FEN string (typically the first position of the
// parsed game). Using the game's actual starting FEN rather than always
// stockfish.StartPosition ensures correctness for PGNs with SetUp/FEN headers.
//
// mateIn is non-nil when the engine reports a forced-mate sequence: positive
// means the side to move mates in that many moves, negative means they are
// being mated. Mate scores are also mapped to ±mateScoreSentinel in the
// returned centipawn score so that downstream arithmetic remains meaningful.
// Returns ErrEngineFailure if the engine stream closes without ever producing
// a best move.
func (r *Reviewer) analyzePosition(ctx context.Context, initialFEN string, moves []string) (score int, bestMove string, mateIn *int, err error) {
	pos := stockfish.FENPosition(initialFEN)
	if len(moves) > 0 {
		pos = pos.WithMoves(moves...)
	}

	if setErr := r.engine.SetPosition(pos); setErr != nil {
		return 0, "", nil, &ErrEngineFailure{Cause: setErr, Reason: fmt.Sprintf("set position failed: %s", setErr.Error())}
	}

	ch, goErr := r.engine.Go(ctx, &stockfish.SearchParams{Depth: r.cfg.depth})
	if goErr != nil {
		return 0, "", nil, &ErrEngineFailure{Cause: goErr, Reason: fmt.Sprintf("go command failed: %s", goErr.Error())}
	}

	bestMoveFound := false

	// lastExactScore and lastExactMateIn track the most recent info line that
	// carried an exact (non-bound) score. We prefer exact scores over lowerbound
	// or upperbound values because bound scores are only guaranteed to be one
	// side of the true minimax value; using them would introduce measurement
	// error into per-move CPL and therefore accuracy/game-rating calculations.
	var (
		lastExactScore  int
		lastExactMateIn *int
		hasExact        bool
	)

	for info := range ch {
		if info.IsBestMove {
			bestMove = info.BestMove
			bestMoveFound = true

			break
		}

		// Always track the latest score as the fallback.
		if info.Score.Type == stockfish.ScoreTypeMate {
			n := info.Score.Value
			mateIn = &n
		} else {
			mateIn = nil
		}

		score = normalizeScore(info.Score)

		// Additionally track the last score that was exact so we can prefer it.
		if info.Score.Bound == stockfish.ScoreBoundExact {
			lastExactScore = score
			lastExactMateIn = mateIn
			hasExact = true
		}
	}

	if !bestMoveFound {
		return 0, "", nil, &ErrEngineFailure{Reason: "engine returned no best move"}
	}

	// Prefer the last exact score over any bound score seen last.
	if hasExact {
		return lastExactScore, bestMove, lastExactMateIn, nil
	}

	return score, bestMove, mateIn, nil
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

// ReviewGameFull analyses the provided PGN string and returns a GameResult
// containing per-move analysis and an aggregated GameSummary for both players.
//
// It is equivalent to calling ReviewGame and then Summarize, but player names
// are extracted directly from the PGN headers so the caller does not need to
// parse them separately.
//
// Returns ErrInvalidPGN when the PGN cannot be parsed.
// Returns ErrEngineFailure when communication with the engine fails.
func (r *Reviewer) ReviewGameFull(ctx context.Context, pgn string) (GameResult, error) {
	if r.engine == nil {
		return GameResult{}, &ErrEngineFailure{Reason: "reviewer not initialized; use New()"}
	}

	gi, err := parsePGN(pgn)
	if err != nil {
		return GameResult{}, err
	}

	reviews, err := r.reviewFromGameInfo(ctx, &gi, nil)
	if err != nil {
		return GameResult{}, err
	}

	summary := Summarize(reviews, gi.WhitePlayer, gi.BlackPlayer, gi.OpeningCode, gi.OpeningTitle)

	return GameResult{Reviews: reviews, Summary: summary}, nil
}

// ReviewGameStream analyses the provided PGN string and streams each MoveReview
// to the returned channel as soon as it is computed. The moves channel is closed
// when all moves have been processed or when an error occurs.
//
// Any engine or parse error is sent on the separate error channel (buffered,
// capacity 1), which is closed after at most one value.
//
// Callers must keep receiving from moves (or cancel the context) until it is
// closed to avoid blocking the background goroutine. Reading from errs is
// recommended for correctness but is not required to prevent blocking, as the
// error channel is buffered.
//
// PGN games with a custom starting position (SetUp/FEN headers) are fully
// supported.
func (r *Reviewer) ReviewGameStream(ctx context.Context, pgn string) (moves <-chan MoveReview, errs <-chan error) {
	movesCh := make(chan MoveReview)
	errsCh := make(chan error, 1)

	if r.engine == nil {
		errsCh <- &ErrEngineFailure{Reason: "reviewer not initialized; use New()"}

		close(errsCh)
		close(movesCh)

		return movesCh, errsCh
	}

	gi, err := parsePGN(pgn)
	if err != nil {
		errsCh <- err

		close(errsCh)
		close(movesCh)

		return movesCh, errsCh
	}

	go func() {
		defer close(movesCh)
		defer close(errsCh)

		if _, runErr := r.reviewFromGameInfo(ctx, &gi, movesCh); runErr != nil {
			errsCh <- runErr
		}
	}()

	return movesCh, errsCh
}

// ReviewGameFullStream analyses the provided PGN string and streams each
// MoveReview to the returned moves channel as soon as it is computed. Once all
// moves have been processed, an aggregated GameSummary is sent on the summary
// channel and both channels are closed.
//
// Any engine or parse error is sent on the separate error channel (buffered,
// capacity 1), which is closed after at most one value. When an error occurs
// the summary channel is closed without a value.
//
// Callers must keep receiving from moves (or cancel the context) until it is
// closed to avoid blocking the background goroutine. The errs and summaries
// channels are buffered (capacity 1) and will not block the producer; reading
// them is recommended for correctness but is not required to prevent blocking.
//
// PGN games with a custom starting position (SetUp/FEN headers) are fully
// supported.
func (r *Reviewer) ReviewGameFullStream(ctx context.Context, pgn string) (moves <-chan MoveReview, errs <-chan error, summaries <-chan GameSummary) {
	movesCh := make(chan MoveReview)
	errsCh := make(chan error, 1)
	summariesCh := make(chan GameSummary, 1)

	if r.engine == nil {
		errsCh <- &ErrEngineFailure{Reason: "reviewer not initialized; use New()"}

		close(errsCh)
		close(movesCh)
		close(summariesCh)

		return movesCh, errsCh, summariesCh
	}

	gi, err := parsePGN(pgn)
	if err != nil {
		errsCh <- err

		close(errsCh)
		close(movesCh)
		close(summariesCh)

		return movesCh, errsCh, summariesCh
	}

	go func() {
		defer close(movesCh)
		defer close(errsCh)
		defer close(summariesCh)

		// reviewFromGameInfo streams each review to movesCh and also returns the
		// full collected slice, which we use to compute the summary afterwards.
		reviews, runErr := r.reviewFromGameInfo(ctx, &gi, movesCh)
		if runErr != nil {
			errsCh <- runErr

			return
		}

		summary := Summarize(reviews, gi.WhitePlayer, gi.BlackPlayer, gi.OpeningCode, gi.OpeningTitle)
		summariesCh <- summary
	}()

	return movesCh, errsCh, summariesCh
}
