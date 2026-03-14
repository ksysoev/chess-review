# Stage 1 – build the chess-review binary
FROM golang:1.26-bookworm AS builder

WORKDIR /src

# Download dependencies first (leverages layer cache)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a statically-linked binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /chess-review ./cmd/chess-review

# Stage 2 – minimal runtime image with Stockfish
FROM debian:bookworm-slim AS runtime

# Install Stockfish from the Debian repository.
# stockfish binary lands at /usr/games/stockfish on Debian.
RUN apt-get update \
    && apt-get install -y --no-install-recommends stockfish \
    && rm -rf /var/lib/apt/lists/*

# Copy the compiled CLI from the builder stage
COPY --from=builder /chess-review /usr/local/bin/chess-review

# Tell the CLI where Stockfish lives
ENV STOCKFISH_PATH=/usr/games/stockfish

ENTRYPOINT ["chess-review"]
