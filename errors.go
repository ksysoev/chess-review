// Package chessreview provides chess game analysis powered by the Stockfish engine.
// It accepts a PGN game string, evaluates each position, and returns a per-move
// review with quality classifications for each move.
//
// Each move is classified into one of seven categories: Best, Excellent, Good,
// Inaccuracy, Mistake, Blunder, or Miss (missed forced mate).
package chessreview

import "fmt"

// ErrInvalidPGN is returned when the provided PGN string cannot be parsed.
type ErrInvalidPGN struct {
	// Cause is the underlying error, if any.
	Cause  error
	Reason string
}

// Error implements the error interface.
func (e *ErrInvalidPGN) Error() string {
	return fmt.Sprintf("invalid PGN: %s", e.Reason)
}

// Unwrap returns the underlying cause so callers can use errors.Is/As.
func (e *ErrInvalidPGN) Unwrap() error {
	return e.Cause
}

// ErrEngineFailure is returned when communication with the Stockfish engine fails.
type ErrEngineFailure struct {
	// Cause is the underlying error, if any.
	Cause  error
	Reason string
}

// Error implements the error interface.
func (e *ErrEngineFailure) Error() string {
	return fmt.Sprintf("engine failure: %s", e.Reason)
}

// Unwrap returns the underlying cause so callers can use errors.Is/As.
func (e *ErrEngineFailure) Unwrap() error {
	return e.Cause
}
