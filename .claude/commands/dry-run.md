# /dry-run

Runs the full Beacon pipeline end-to-end — mood detection, paper fetching, briefing generation — but **never sends anything**. Output is printed to stdout. Use this before deploying or after changing prompts to verify the output quality.

## What it does

1. Loads config from `.env` with `DRY_RUN=true` forced
2. Detects current mood via Spotify
3. Fetches papers from all configured sources
4. Applies topic filters
5. Generates briefings via Claude API (real API call — tokens are consumed)
6. Prints the full briefing to stdout
7. Prints a summary report at the end
8. Exits — Telegram and email delivery are completely skipped

## Expected output

```
=== Beacon / dry run ===

[mood]    HIGH_BPM (BPM: 148 · genre: trap)
[papers]  fetched 12 papers across 4 sources
          ArXiv: 6 · HuggingFace: 2 · Semantic Scholar: 3 · bioRxiv: 1
[filter]  8 papers passed topic filter
[mode]    SUMMARY (HIGH_BPM)

──────────────────────────────────────────

📄 Scaling Laws for Neural Language Models
🏷️  Kaplan et al. · ArXiv cs.LG · 2026-03-20
⚡ Larger models trained on less data outperform smaller models trained longer.
   Compute-optimal training requires scaling model size and data proportionally.

──────────────────────────────────────────

📄 BCI-GPT: Decoding Motor Imagery with Foundation Models
🏷️  Zhang et al. · bioRxiv · 2026-03-19
⚡ Fine-tuned GPT-4 architecture decodes EEG motor imagery at 94.2% accuracy,
   beating prior SOTA by 6.1% with 10x less labeled data.

──────────────────────────────────────────

[... remaining 6 papers ...]

──────────────────────────────────────────

=== Summary ===
Papers generated:  8
Claude API calls:  8
Tokens consumed:   ~2,400
Delivery:          SKIPPED (dry run)
Duration:          14.2s
```

## How to run

```bash
make dry-run
# or directly:
DRY_RUN=true go run cmd/beacon/main.go --cmd=run
# or override mood for testing:
DRY_RUN=true MOCK_MOOD=HIGH_BPM go run cmd/beacon/main.go --cmd=run
DRY_RUN=true MOCK_MOOD=NORMAL go run cmd/beacon/main.go --cmd=run
```

## MOCK_MOOD override

When `MOCK_MOOD` is set, the detector skips Spotify and returns the specified mood directly. Useful for testing both code paths without needing Spotify active.

```
MOCK_MOOD=HIGH_BPM  → forces HIGH_BPM regardless of what's playing
MOCK_MOOD=NORMAL    → forces NORMAL
(unset)             → uses real Spotify detection
```

## Implementation notes for the Go Dev agent

- `DRY_RUN=true` must be checked in every delivery function before any network call
- All delivery functions must log `[dry run] skipping send` at info level
- The summary report at the end must always print, even on partial failure
- Token consumption estimate: 300 tokens × N papers (HIGH_BPM) or 1500 × N (NORMAL)
- Exit code 0 on success, 1 on config error, any paper/Claude failure is logged but non-fatal

## Key invariant

**`DRY_RUN=true` must be respected even if the user explicitly passes `--force-send`.**
There is no override for dry run mode — it is a safety guarantee, not a preference.