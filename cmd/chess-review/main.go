// Package main is the entry point for the chess-review CLI.
// It reads a PGN file, analyses every half-move using the Stockfish engine,
// and prints a per-move review table followed by a game summary to standard output.
// Move information is printed as soon as each position is analysed, so the user
// can follow progress without waiting for the full game to complete.
package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"

	chessreview "github.com/ksysoev/chess-review"
	"github.com/spf13/cobra"
)

const (
	defaultStockfishPath = "/usr/games/stockfish"
	envStockfishPath     = "STOCKFISH_PATH"

	flagDepth        = "depth"
	flagDepthDefault = chessreview.DefaultDepth
	flagDepthUsage   = "Stockfish search depth (higher = stronger but slower, default 18)"

	flagThreads        = "threads"
	flagThreadsDefault = chessreview.DefaultThreads
	flagThreadsUsage   = "Number of CPU threads Stockfish may use (default 1)"

	flagHash        = "hash"
	flagHashDefault = chessreview.DefaultHashMB
	flagHashUsage   = "Stockfish transposition table size in MB (default 16)"

	flagTopMoves        = "top-moves"
	flagTopMovesDefault = chessreview.DefaultTopMoves
	flagTopMovesUsage   = "Number of candidate moves to evaluate per position (default 3)"

	// Column widths for the fixed-format streaming move table.
	colMove           = 4
	colColor          = 5
	colMoveUCI        = 6
	colClassification = 14
	colMateBefore     = 11
	colMateAfter      = 10
	colScoreBefore    = 12
	colScoreAfter     = 11
	colDelta          = 7
	colTopMoves       = 32
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

Move information is printed as soon as each position is analysed; you do not
need to wait for the entire game to be processed before seeing results.

The Stockfish binary path is read from the STOCKFISH_PATH environment variable
(default: /usr/games/stockfish).

Example:
  chess-review game.pgn
  chess-review --depth 20 game.pgn
  chess-review --depth 20 --threads 4 --hash 128 game.pgn
  chess-review --top-moves 5 game.pgn
  STOCKFISH_PATH=/usr/local/bin/stockfish chess-review game.pgn`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         run,
	}

	cmd.Flags().Int(flagDepth, flagDepthDefault, flagDepthUsage)
	cmd.Flags().Int(flagThreads, flagThreadsDefault, flagThreadsUsage)
	cmd.Flags().Int(flagHash, flagHashDefault, flagHashUsage)
	cmd.Flags().Int(flagTopMoves, flagTopMovesDefault, flagTopMovesUsage)

	return cmd
}

// run is the cobra command handler. It orchestrates reading the PGN file,
// constructing the Reviewer, and streaming the analysis table and game summary.
func run(cmd *cobra.Command, args []string) error {
	pgnPath := args[0]

	depth, err := cmd.Flags().GetInt(flagDepth)
	if err != nil {
		return fmt.Errorf("reading flag --%s: %w", flagDepth, err)
	}

	threads, err := cmd.Flags().GetInt(flagThreads)
	if err != nil {
		return fmt.Errorf("reading flag --%s: %w", flagThreads, err)
	}

	hash, err := cmd.Flags().GetInt(flagHash)
	if err != nil {
		return fmt.Errorf("reading flag --%s: %w", flagHash, err)
	}

	topMoves, err := cmd.Flags().GetInt(flagTopMoves)
	if err != nil {
		return fmt.Errorf("reading flag --%s: %w", flagTopMoves, err)
	}

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

	reviewer, err := chessreview.New(stockfishPath, chessreview.WithDepth(depth), chessreview.WithThreads(threads), chessreview.WithHash(hash), chessreview.WithTopMoves(topMoves))
	if err != nil {
		return fmt.Errorf("starting engine: %w", err)
	}

	defer func() {
		if closeErr := reviewer.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: closing engine: %v\n", closeErr)
		}
	}()

	movesCh, errCh, summariesCh := reviewer.ReviewGameFullStream(ctx, string(pgnBytes))

	printTableHeader()

	for mr := range movesCh {
		printTableRow(&mr)
	}

	if streamErr := <-errCh; streamErr != nil {
		return fmt.Errorf("reviewing game: %w", streamErr)
	}

	summary, ok := <-summariesCh
	if !ok {
		// errCh carried no error yet the summary channel was closed without a
		// value — this violates the stream contract and should never happen.
		return fmt.Errorf("reviewing game: stream closed without a summary")
	}

	fmt.Fprintln(os.Stdout)
	printSummary(&summary)

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

// formatTopMoves formats a slice of MoveEvaluation as a compact string for
// display in the CLI table, e.g. "e2e4(+50) d2d4(+20) g1f3(-5)".
// Each entry shows the UCI move followed by its signed centipawn score in
// parentheses. Mate scores are shown as "M<N>" or "-M<N>".
func formatTopMoves(moves []chessreview.MoveEvaluation) string {
	if len(moves) == 0 {
		return "-"
	}

	parts := make([]string, 0, len(moves))

	for _, m := range moves {
		var scoreStr string

		if m.MateIn != nil {
			if *m.MateIn >= 0 {
				scoreStr = fmt.Sprintf("M%d", *m.MateIn)
			} else {
				scoreStr = fmt.Sprintf("-M%d", -*m.MateIn)
			}
		} else {
			scoreStr = fmt.Sprintf("%+d", m.Score)
		}

		parts = append(parts, fmt.Sprintf("%s(%s)", m.Move, scoreStr))
	}

	return strings.Join(parts, " ")
}

// printTableHeader writes the fixed-width column headers for the move table.
func printTableHeader() {
	fmt.Fprintf(os.Stdout,
		"%-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %-*s\n",
		colMove, "Move",
		colColor, "Color",
		colMoveUCI, "Played",
		colClassification, "Classification",
		colMateBefore, "Mate Before",
		colMateAfter, "Mate After",
		colScoreBefore, "Score Before",
		colScoreAfter, "Score After",
		colDelta, "Delta",
		colTopMoves, "Top Moves",
	)
	fmt.Fprintf(os.Stdout,
		"%-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %-*s\n",
		colMove, "----",
		colColor, "-----",
		colMoveUCI, "------",
		colClassification, "--------------",
		colMateBefore, "-----------",
		colMateAfter, "----------",
		colScoreBefore, "------------",
		colScoreAfter, "-----------",
		colDelta, "-------",
		colTopMoves, "--------------------------------",
	)
}

// printTableRow writes a single move review as a fixed-width row to stdout.
func printTableRow(r *chessreview.MoveReview) {
	// Truncate the Top Moves string so the column never exceeds colTopMoves
	// characters.  %-*s pads but does not truncate, so a long value (e.g. when
	// --top-moves is large) would overflow the fixed-width column.
	top := formatTopMoves(r.TopMoves)
	if len(top) > colTopMoves {
		top = top[:colTopMoves]
	}

	fmt.Fprintf(os.Stdout,
		"%-*d  %-*s  %-*s  %-*s  %-*s  %-*s  %-*d  %-*d  %+*d  %-*s\n",
		colMove, r.MoveNumber,
		colColor, r.Color,
		colMoveUCI, r.PlayedMove,
		colClassification, r.Classification,
		colMateBefore, formatMateIn(r.MateInBefore),
		colMateAfter, formatMateIn(r.MateInAfter),
		colScoreBefore, r.ScoreBefore,
		colScoreAfter, r.ScoreAfter,
		colDelta, r.ScoreDelta,
		colTopMoves, top,
	)
}

// formatAccuracy formats an accuracy float as "XX.X%" or "-" for NaN.
func formatAccuracy(acc float64) string {
	if math.IsNaN(acc) {
		return "-"
	}

	return fmt.Sprintf("%.1f%%", acc)
}

// printSummary writes the aggregated game summary as a two-column
// (White | Black) fixed-width table to standard output.
func printSummary(s *chessreview.GameSummary) {
	whiteName := s.WhitePlayer
	if whiteName == "" {
		whiteName = "White"
	}

	blackName := s.BlackPlayer
	if blackName == "" {
		blackName = "Black"
	}

	const (
		colLabel = 14
		colValue = 10
	)

	row := func(label, white, black string) {
		fmt.Fprintf(os.Stdout, "%-*s  %-*s  %-*s\n", colLabel, label, colValue, white, colValue, black)
	}

	row("Game Summary", whiteName, blackName)
	row("------------", "-------", "-------")

	var opening string

	switch {
	case s.OpeningCode != "" && s.OpeningTitle != "":
		opening = s.OpeningCode + " - " + s.OpeningTitle
	case s.OpeningCode != "":
		opening = s.OpeningCode
	case s.OpeningTitle != "":
		opening = s.OpeningTitle
	}

	if opening != "" {
		row("Opening", opening, opening)
	}

	row("Accuracy", formatAccuracy(s.White.Accuracy), formatAccuracy(s.Black.Accuracy))
	row("Game Rating", fmt.Sprintf("%d", s.White.GameRating), fmt.Sprintf("%d", s.Black.GameRating))

	fmt.Fprintln(os.Stdout)

	classifications := []chessreview.Classification{
		chessreview.Book,
		chessreview.Brilliant,
		chessreview.Great,
		chessreview.Best,
		chessreview.Excellent,
		chessreview.Good,
		chessreview.Inaccuracy,
		chessreview.Mistake,
		chessreview.Blunder,
		chessreview.Miss,
	}

	for _, c := range classifications {
		row(c.String(), fmt.Sprintf("%d", s.White.ClassificationCounts[c]), fmt.Sprintf("%d", s.Black.ClassificationCounts[c]))
	}

	fmt.Fprintln(os.Stdout)

	phases := []struct {
		name  string
		phase chessreview.GamePhase
	}{
		{"Opening", chessreview.Opening},
		{"Middlegame", chessreview.Middlegame},
		{"Endgame", chessreview.Endgame},
	}

	for _, p := range phases {
		row(p.name, formatAccuracy(s.White.PhaseAccuracy[p.phase]), formatAccuracy(s.Black.PhaseAccuracy[p.phase]))
	}
}
