# /paper-fetch

Fetches papers from all configured sources for a given topic and prints the raw results. No mood detection, no Claude API, no delivery. Use this to debug sources, verify topic filters, and inspect what Beacon actually pulls before briefing generation.

## What it does

1. Loads config from `.env`
2. Queries all 4 sources for the specified topic (or all topics if none given)
3. Applies deduplication
4. Prints raw paper metadata to stdout
5. Exits — no filtering beyond dedup, no Claude calls, nothing sent

## How to run

```bash
# Fetch all topics
make paper-fetch

# Fetch specific topic
go run cmd/beacon/main.go --cmd=fetch --topic="AI/ML"
go run cmd/beacon/main.go --cmd=fetch --topic="BCI"
go run cmd/beacon/main.go --cmd=fetch --topic="Healthcare AI"
go run cmd/beacon/main.go --cmd=fetch --topic="Computer Vision"
go run cmd/beacon/main.go --cmd=fetch --topic="Bioengineering"
go run cmd/beacon/main.go --cmd=fetch --topic="Anthropic"

# Fetch from one source only (for debugging)
go run cmd/beacon/main.go --cmd=fetch --topic="AI/ML" --source=arxiv
go run cmd/beacon/main.go --cmd=fetch --topic="AI/ML" --source=huggingface
go run cmd/beacon/main.go --cmd=fetch --topic="AI/ML" --source=semantic_scholar
go run cmd/beacon/main.go --cmd=fetch --topic="BCI"   --source=biorxiv
```

## Expected output

```
=== Beacon / paper fetch ===
Topic:   AI/ML
Sources: arxiv, huggingface, semantic_scholar, biorxiv

[arxiv]           fetched 8 papers (2.1s)
[huggingface]     fetched 3 papers (0.8s)
[semantic_scholar] fetched 5 papers (1.4s)
[biorxiv]         fetched 0 papers (0.3s)

After dedup: 14 papers (2 duplicates removed)

──────────────────────────────────────────
 1. Efficient Attention: A Survey
    Authors:  Tay et al.
    Source:   ArXiv cs.LG
    Date:     2026-03-21
    URL:      https://arxiv.org/abs/2603.12345
    Abstract: We survey efficient attention mechanisms...
    Passed filter: YES (keywords: attention, transformer)

──────────────────────────────────────────
 2. Mamba-2: Linear-Time Sequence Modeling
    Authors:  Gu, Dao
    Source:   HuggingFace Papers
    Date:     2026-03-20
    URL:      https://huggingface.co/papers/2603.09876
    Abstract: State space models offer linear complexity...
    Passed filter: YES (keywords: sequence modeling, LLM)

──────────────────────────────────────────

=== Summary ===
Total fetched:    16
Duplicates:        2
Passed filter:    14
Failed filter:     0
Duration:         4.6s
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--topic` | string | all topics | Topic name from config (must match exactly) |
| `--source` | string | all sources | One of: `arxiv`, `huggingface`, `semantic_scholar`, `biorxiv` |
| `--limit` | int | 10 per source | Max papers per source |
| `--no-filter` | bool | false | Skip topic keyword filter, show all raw results |
| `--json` | bool | false | Output as JSON array instead of human-readable |

## Implementation notes for the Go Dev agent

- Sources must be fetched concurrently — each source is independent
- Use `errgroup` for concurrent fetching with context cancellation
- Dedup by URL (primary) then by title similarity (secondary — avoid exact reposts)
- `--json` output must be valid JSON: array of `papers.Paper` structs
- Per-source timing must be printed — useful for debugging slow sources
- Exit code 0 on success, 1 on config error
- Partial failure is OK: if one source fails, log it and continue with the others

## Common failure modes

| Error | Likely cause | Fix |
|-------|-------------|-----|
| `arxiv: timeout after 10s` | ArXiv rate limiting | Add delay, reduce `--limit` |
| `semantic_scholar: 429` | API rate limit | Wait 60s, retry |
| `huggingface: 0 papers` | Feed format changed | Check HF Papers page manually, update parser |
| `biorxiv: parse error` | RSS schema change | Update `biorxiv.go` parser |