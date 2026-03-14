# chess-review

[![Tests](https://github.com/ksysoev/chess-review/actions/workflows/tests.yml/badge.svg)](https://github.com/ksysoev/chess-review/actions/workflows/tests.yml)
[![codecov](https://codecov.io/gh/ksysoev/chess-review/graph/badge.svg?token=QFIJMDU6FZ)](https://codecov.io/gh/ksysoev/chess-review)
[![Go Report Card](https://goreportcard.com/badge/github.com/ksysoev/chess-review)](https://goreportcard.com/report/github.com/ksysoev/chess-review)
[![Go Reference](https://pkg.go.dev/badge/github.com/ksysoev/chess-review.svg)](https://pkg.go.dev/github.com/ksysoev/chess-review)


A Go library for analyzing chess games using the [Stockfish](https://stockfishchess.org/) engine. It parses a PGN game, evaluates each position at a configurable depth, and returns a per-move review with move-quality classifications — similar to chess.com's game review feature.

## Features

- Parse PGN input via [`github.com/notnil/chess`](https://github.com/notnil/chess)
- Evaluate each position with Stockfish (N+1 engine calls for N plies — no redundant work)
- Classify moves as **Best**, **Excellent**, **Good**, **Inaccuracy**, **Mistake**, **Blunder**, or **Miss** (missed forced mate)
- Configurable search depth, thread count, and hash table size
- Typed errors with full `errors.Is`/`errors.As` chain support

## Requirements

- Go 1.21+
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
        fmt.Printf("Move %d (%s) %s → %s [%s]\n",
            r.MoveNumber, r.Color, r.PlayedMove, r.BestMove, r.Classification)
    }
}
```

### Example output

```
Move 1 (white) e2e4 → e2e4 [Best]
Move 1 (black) e7e5 → e7e5 [Best]
Move 2 (white) g1f3 → g1f3 [Best]
Move 2 (black) b8c6 → b8c6 [Best]
Move 3 (white) f1c4 → f1c4 [Best]
Move 3 (black) f8c5 → f8c5 [Best]
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

| Field            | Type             | Description                                              |
|------------------|------------------|----------------------------------------------------------|
| `PlayedMove`     | `string`         | The move that was played, in UCI notation (e.g. `e2e4`) |
| `BestMove`       | `string`         | Engine's top recommendation at the configured depth      |
| `Color`          | `string`         | `"white"` or `"black"`                                   |
| `MoveNumber`     | `int`            | Full-move number (1-indexed)                             |
| `Classification` | `Classification` | Quality rating of the played move                        |
| `ScoreBefore`    | `int`            | Centipawn score before the move (side-to-move frame)     |
| `ScoreAfter`     | `int`            | Centipawn score after the move (side-to-move frame)      |
| `ScoreDelta`     | `int`            | `ScoreAfter − ScoreBefore`; negative means loss          |

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

| Option                  | Default | Description                              |
|-------------------------|---------|------------------------------------------|
| `WithDepth(n int)`      | `18`    | Stockfish search depth (must be ≥ 1)     |
| `WithThreads(n int)`    | `1`     | CPU threads for the engine (must be ≥ 1) |
| `WithHash(mb int)`      | `16`    | Transposition table size in MB (≥ 1)     |

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

[MIT](LICENSE)
