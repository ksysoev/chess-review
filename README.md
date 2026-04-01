# chess-review

[![Tests](https://github.com/ksysoev/chess-review/actions/workflows/tests.yml/badge.svg)](https://github.com/ksysoev/chess-review/actions/workflows/tests.yml)
[![codecov](https://codecov.io/gh/ksysoev/chess-review/graph/badge.svg?token=QFIJMDU6FZ)](https://codecov.io/gh/ksysoev/chess-review)
[![Go Report Card](https://goreportcard.com/badge/github.com/ksysoev/chess-review)](https://goreportcard.com/report/github.com/ksysoev/chess-review)
[![Go Reference](https://pkg.go.dev/badge/github.com/ksysoev/chess-review.svg)](https://pkg.go.dev/github.com/ksysoev/chess-review)


A Go library for analyzing chess games using the [Stockfish](https://stockfishchess.org/) engine. It parses a PGN game, evaluates each position at a configurable depth, and returns a per-move review with move-quality classifications — similar to chess.com's game review feature.

## Features

- Parse PGN input via [`github.com/notnil/chess`](https://github.com/notnil/chess)
- Evaluate each position with Stockfish (N+1 engine calls for N plies — no redundant work)
- Return the top N candidate moves (MultiPV) for each position with their centipawn scores
- Classify moves as **Best**, **Excellent**, **Good**, **Inaccuracy**, **Mistake**, **Blunder**, or **Miss** (missed forced mate)
- Configurable search depth, thread count, hash table size, and number of candidate moves
- Typed errors with full `errors.Is`/`errors.As` chain support

## Requirements

- Go 1.26+
- A [Stockfish](https://stockfishchess.org/download/) binary accessible on the system

## Installation

```sh
go get github.com/ksysoev/chess-review
```

## Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    "strings"

    chessreview "github.com/ksysoev/chess-review"
)

func main() {
    pgn := `[Event "Example"]
[White "Alice"]
[Black "Bob"]
[Result "*"]

1. e4 e5 2. Nf3 Nc6 3. Bc4 Bc5 *`

    reviewer, err := chessreview.New("/usr/local/bin/stockfish",
        chessreview.WithDepth(18),
        chessreview.WithThreads(2),
        chessreview.WithHash(64),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer reviewer.Close()

    reviews, err := reviewer.ReviewGame(context.Background(), pgn)
    if err != nil {
        log.Fatal(err)
    }

    for _, r := range reviews {
        bestMove := ""
        topParts := make([]string, 0, len(r.TopMoves))

        for _, m := range r.TopMoves {
            if bestMove == "" {
                bestMove = m.Move
            }

            if m.MateIn != nil {
                topParts = append(topParts, fmt.Sprintf("%s(M%d)", m.Move, *m.MateIn))
            } else {
                topParts = append(topParts, fmt.Sprintf("%s(%+d)", m.Move, m.Score))
            }
        }

        fmt.Printf("Move %d (%s) %s → %s [%s]   top: %s\n",
            r.MoveNumber, r.Color, r.PlayedMove, bestMove, r.Classification,
            strings.Join(topParts, " "))
    }
}
```

### Example output

```
Move 1 (white) e2e4 → e2e4 [Best]   top: e2e4(+18) d2d4(+15) g1f3(+10)
Move 1 (black) e7e5 → e7e5 [Best]   top: e7e5(+0) c7c5(-5) e7e6(-8)
Move 2 (white) g1f3 → g1f3 [Best]   top: g1f3(+25) f1c4(+22) b1c3(+18)
Move 2 (black) b8c6 → b8c6 [Best]   top: b8c6(+0) g8f6(-3) d7d6(-6)
Move 3 (white) f1c4 → f1c4 [Best]   top: f1c4(+30) f1b5(+28) d2d4(+20)
Move 3 (black) f8c5 → f8c5 [Best]   top: f8c5(+0) g8f6(-4) d7d6(-7)
```

## API

### `New(stockfishPath string, opts ...Option) (*Reviewer, error)`

Creates a new `Reviewer` backed by the Stockfish binary at `stockfishPath`. Returns `*ErrEngineFailure` if the engine cannot be started or the options are invalid.

### `(*Reviewer) ReviewGame(ctx context.Context, pgn string) ([]MoveReview, error)`

Analyses every half-move in the PGN and returns a `MoveReview` slice. Errors:

- `*ErrInvalidPGN` — PGN cannot be parsed or contains no moves
- `*ErrEngineFailure` — communication with Stockfish failed

### `(*Reviewer) Close() error`

Shuts down the underlying Stockfish process.

### `MoveReview`

| Field            | Type                  | Description                                              |
|------------------|-----------------------|----------------------------------------------------------|
| `PlayedMove`     | `string`              | The move that was played, in UCI notation (e.g. `e2e4`) |
| `TopMoves`       | `[]MoveEvaluation`    | Engine's top N candidate moves at the configured depth   |
| `Color`          | `string`              | `"white"` or `"black"`                                   |
| `MoveNumber`     | `int`                 | Full-move number (1-indexed)                             |
| `Classification` | `Classification`      | Quality rating of the played move                        |
| `ScoreBefore`    | `int`                 | Centipawn score before the move (side-to-move frame)     |
| `ScoreAfter`     | `int`                 | Centipawn score after the move, from the perspective of the side that just moved (negated from the engine's output so both `ScoreBefore` and `ScoreAfter` share the same reference frame) |
| `ScoreDelta`     | `int`                 | `ScoreAfter − ScoreBefore`; negative means loss          |

### `MoveEvaluation`

| Field    | Type    | Description                                                       |
|----------|---------|-------------------------------------------------------------------|
| `Move`   | `string`| Candidate move in UCI notation (e.g. `e2e4`)                     |
| `Score`  | `int`   | Centipawn evaluation from the side-to-move perspective            |
| `MateIn` | `*int`  | Moves to forced mate (`nil` if no forced mate; negative = mated)  |

### Move classifications

| Classification | Centipawn loss     |
|----------------|--------------------|
| `Best`         | Played == engine best |
| `Excellent`    | 0–10 cp            |
| `Good`         | 11–25 cp           |
| `Inaccuracy`   | 26–100 cp          |
| `Mistake`      | 101–300 cp         |
| `Blunder`      | > 300 cp           |
| `Miss`         | Missed forced mate (sentinel loss ≥ 20 000 cp) |

### Functional options

| Option                  | Default | Description                                              |
|-------------------------|---------|----------------------------------------------------------|
| `WithDepth(n int)`      | `18`    | Stockfish search depth (must be ≥ 1)                     |
| `WithThreads(n int)`    | `1`     | CPU threads for the engine (must be ≥ 1)                 |
| `WithHash(mb int)`      | `16`    | Transposition table size in MB (≥ 1)                     |
| `WithTopMoves(n int)`   | `3`     | Number of candidate moves to evaluate per position (≥ 1) |

## Development

```sh
make test    # run tests with race detector
make lint    # run golangci-lint
make fmt     # format with gofmt
make tidy    # go mod tidy
make mocks   # regenerate mocks with mockery
make fields  # fix struct field alignment
```

## License

[GPL 2.0](LICENSE)
