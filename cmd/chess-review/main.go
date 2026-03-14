// Package main is the entry point for the chess-review CLI.
// It reads a PGN file, analyses every half-move using the Stockfish engine,
// and prints a per-move review table to standard output.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	chessreview "github.com/ksysoev/chess-review"
	"github.com/spf13/cobra"
)

const (
	defaultStockfishPath = "/usr/games/stockfish"
	envStockfishPath     = "STOCKFISH_PATH"

	tabWriterMinWidth = 0
	tabWriterTabWidth = 0
	tabWriterPadding  = 2
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// newRootCmd builds and returns the cobra root command.
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chess-review <pgn-file>",
		Short: "Analyse a chess game PGN file using Stockfish",
		Long: `chess-review reads a PGN file and produces a per-move analysis table.

The Stockfish binary path is read from the STOCKFISH_PATH environment variable
(default: /usr/games/stockfish).

Example:
  chess-review game.pgn
  STOCKFISH_PATH=/usr/local/bin/stockfish chess-review game.pgn`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         run,
	}

	return cmd
}

// run is the cobra command handler. It orchestrates reading the PGN file,
// constructing the Reviewer, and printing the analysis table.
func run(_ *cobra.Command, args []string) error {
	pgnPath := args[0]

	stockfishPath := os.Getenv(envStockfishPath)
	if stockfishPath == "" {
		stockfishPath = defaultStockfishPath
	}

	pgnBytes, err := os.ReadFile(pgnPath)
	if err != nil {
		return fmt.Errorf("reading PGN file: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	reviewer, err := chessreview.New(stockfishPath)
	if err != nil {
		return fmt.Errorf("starting engine: %w", err)
	}

	defer func() {
		if closeErr := reviewer.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: closing engine: %v\n", closeErr)
		}
	}()

	reviews, err := reviewer.ReviewGame(ctx, string(pgnBytes))
	if err != nil {
		return fmt.Errorf("reviewing game: %w", err)
	}

	printTable(reviews)

	return nil
}

// formatMateIn formats a MateIn pointer as a display string.
// Returns "M<N>" for forced mate, "-M<N>" for being mated, or "-" when nil.
func formatMateIn(mateIn *int) string {
	if mateIn == nil {
		return "-"
	}

	if *mateIn >= 0 {
		return fmt.Sprintf("M%d", *mateIn)
	}

	return fmt.Sprintf("-M%d", -*mateIn)
}

// printTable writes the move review slice as a human-readable tab-aligned table
// to standard output.
func printTable(reviews []chessreview.MoveReview) {
	w := tabwriter.NewWriter(os.Stdout, tabWriterMinWidth, tabWriterTabWidth, tabWriterPadding, ' ', 0)

	fmt.Fprintln(w, "Move\tColor\tPlayed\tBest\tClassification\tMate Before\tMate After\tScore Before\tScore After\tDelta")
	fmt.Fprintln(w, "----\t-----\t------\t----\t--------------\t-----------\t----------\t------------\t-----------\t-----")

	for _, r := range reviews {
		fmt.Fprintf(
			w,
			"%d\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\t%+d\n",
			r.MoveNumber,
			r.Color,
			r.PlayedMove,
			r.BestMove,
			r.Classification,
			formatMateIn(r.MateInBefore),
			formatMateIn(r.MateInAfter),
			r.ScoreBefore,
			r.ScoreAfter,
			r.ScoreDelta,
		)
	}

	_ = w.Flush()
}
