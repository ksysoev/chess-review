// Package main is the entry point for the chess-review CLI.
// It reads a PGN file, analyses every half-move using the Stockfish engine,
// and prints a per-move review table followed by a game summary to standard output.
package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/signal"
	"runtime"
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
		Long: `chess-review reads a PGN file and produces a per-move analysis table
followed by an aggregated game summary.

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
// constructing the Reviewer, and printing the analysis table and game summary.
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

	reviewer, err := chessreview.New(stockfishPath, chessreview.WithThreads(runtime.NumCPU()))
	if err != nil {
		return fmt.Errorf("starting engine: %w", err)
	}

	defer func() {
		if closeErr := reviewer.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: closing engine: %v\n", closeErr)
		}
	}()

	result, err := reviewer.ReviewGameFull(ctx, string(pgnBytes))
	if err != nil {
		return fmt.Errorf("reviewing game: %w", err)
	}

	printTable(result.Reviews)
	fmt.Fprintln(os.Stdout)
	printSummary(&result.Summary)

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

// formatAccuracy formats an accuracy float as "XX.X%" or "-" for NaN.
func formatAccuracy(acc float64) string {
	if math.IsNaN(acc) {
		return "-"
	}

	return fmt.Sprintf("%.1f%%", acc)
}

// printSummary writes the aggregated game summary as a two-column
// (White | Black) tab-aligned table to standard output.
func printSummary(s *chessreview.GameSummary) {
	w := tabwriter.NewWriter(os.Stdout, tabWriterMinWidth, tabWriterTabWidth, tabWriterPadding, ' ', 0)

	whiteName := s.WhitePlayer
	if whiteName == "" {
		whiteName = "White"
	}

	blackName := s.BlackPlayer
	if blackName == "" {
		blackName = "Black"
	}

	fmt.Fprintf(w, "Game Summary\t%s\t%s\n", whiteName, blackName)
	fmt.Fprintln(w, "------------\t-------\t-------")

	fmt.Fprintf(w, "Accuracy\t%s\t%s\n",
		formatAccuracy(s.White.Accuracy),
		formatAccuracy(s.Black.Accuracy))

	fmt.Fprintf(w, "Game Rating\t%d\t%d\n",
		s.White.GameRating,
		s.Black.GameRating)

	fmt.Fprintln(w, "")

	classifications := []chessreview.Classification{
		chessreview.Brilliant,
		chessreview.Best,
		chessreview.Excellent,
		chessreview.Good,
		chessreview.Inaccuracy,
		chessreview.Mistake,
		chessreview.Miss,
		chessreview.Blunder,
	}

	for _, c := range classifications {
		fmt.Fprintf(w, "%s\t%d\t%d\n",
			c,
			s.White.ClassificationCounts[c],
			s.Black.ClassificationCounts[c])
	}

	fmt.Fprintln(w, "")

	phases := []struct {
		name  string
		phase chessreview.GamePhase
	}{
		{"Opening", chessreview.Opening},
		{"Middlegame", chessreview.Middlegame},
		{"Endgame", chessreview.Endgame},
	}

	for _, p := range phases {
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			p.name,
			formatAccuracy(s.White.PhaseAccuracy[p.phase]),
			formatAccuracy(s.Black.PhaseAccuracy[p.phase]))
	}

	_ = w.Flush()
}
