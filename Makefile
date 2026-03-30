.PHONY: run build test lint fmt dry-run test-mood paper-fetch

# Run the scheduler (default command)
run:
	go run cmd/beacon/main.go

# Build binary to bin/beacon
build:
	go build -o bin/beacon cmd/beacon/main.go

# Run all tests with race detector
test:
	go test ./... -race

# Format code (run before committing)
fmt:
	gofmt -w $$(find . -name "*.go" -not -path "./.claude/*")

# Run linter
lint:
	golangci-lint run

# Full pipeline dry run — fetches papers, generates briefing, prints to stdout (no send)
# Use HOUR=21 to simulate the 9pm run. MOCK_MOOD=HIGH_BPM|NORMAL overrides Spotify.
# Use PAPERS=N to limit how many papers Claude processes (default 3, saves API credits).
dry-run:
	go run cmd/beacon/main.go --cmd=dry-run --hour=$(or $(HOUR),9) --papers=$(or $(PAPERS),3)

# Test Spotify mood detection — prints current track, BPM, genres, and MoodLevel
test-mood:
	go run cmd/beacon/main.go --cmd=mood

# Fetch papers from all sources for a given topic
paper-fetch:
	go run cmd/beacon/main.go --cmd=fetch --topic=AI
