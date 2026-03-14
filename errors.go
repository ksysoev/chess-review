// Package chessreview provides chess game analysis powered by the Stockfish engine.
// It accepts a PGN game string, evaluates each position, and returns a per-move
// review similar to chess.com's game review feature.
package chessreview

import "fmt"

// ErrInvalidPGN is returned when the provided PGN string cannot be parsed.
type ErrInvalidPGN struct {
	Reason string
}

// Error implements the error interface.
func (e *ErrInvalidPGN) Error() string {
	return fmt.Sprintf("invalid PGN: %s", e.Reason)
}

// ErrEngineFailure is returned when communication with the Stockfish engine fails.
type ErrEngineFailure struct {
	Reason string
}

// Error implements the error interface.
func (e *ErrEngineFailure) Error() string {
	return fmt.Sprintf("engine failure: %s", e.Reason)
}
