.PHONY: run build test lint dry-run test-mood paper-fetch

# Run the scheduler (default command)
run:
	go run cmd/beacon/main.go

# Build binary to bin/beacon
build:
	go build -o bin/beacon cmd/beacon/main.go

# Run all tests with race detector
test:
	go test ./... -race

# Run linter
lint:
	golangci-lint run

# Full pipeline dry run — fetches papers, generates briefing, prints to stdout (no send)
dry-run:
	DRY_RUN=true go run cmd/beacon/main.go --cmd=run

# Test Spotify mood detection — prints current track, BPM, genres, and MoodLevel
test-mood:
	go run cmd/beacon/main.go --cmd=mood

# Fetch papers from all sources for a given topic
paper-fetch:
	go run cmd/beacon/main.go --cmd=fetch --topic=AI
