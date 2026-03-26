# /test-mood

Hits the Spotify API with your current credentials and prints the detected MoodLevel to stdout. Use this to verify your OAuth token is working and to sanity-check the classifier before a full run.

## What it does

1. Loads config from `.env`
2. Calls `mood/spotify.go` → gets the currently playing track
3. Runs it through `mood/detector.go` → classifies MoodLevel
4. Prints a structured report to stdout
5. Exits — no papers fetched, no briefing generated, nothing sent

## Expected output

```
=== Beacon / mood check ===

Track:   HUMBLE. — Kendrick Lamar
BPM:     151
Genres:  [rap, hip-hop, west coast rap]

Mood:    HIGH_BPM
Reason:  BPM 151 ≥ 140

Spotify token: valid (expires in 3542s)
```

```
=== Beacon / mood check ===

Track:   Clair de Lune — Debussy
BPM:     54
Genres:  [classical, impressionism]

Mood:    NORMAL
Reason:  BPM 54 < 140, no high-energy genres detected

Spotify token: valid (expires in 1821s)
```

```
=== Beacon / mood check ===

Track:   (nothing playing)
Mood:    NORMAL (default — no active playback)

Spotify token: valid (expires in 2910s)
```

## How to run

```bash
make test-mood
# or directly:
go run cmd/beacon/main.go --cmd=mood
```

## Implementation notes for the Go Dev agent

- Entry point: `cmd/beacon/main.go` switches on `--cmd=mood`
- Must print the raw BPM and genre list — not just the final MoodLevel
- Must print token expiry so the developer knows when to re-auth
- Exit code 0 on success, 1 on config error, 2 on Spotify API error
- `DRY_RUN` flag has no effect here — this command never sends anything

## Common failure modes

| Error | Likely cause | Fix |
|-------|-------------|-----|
| `spotify: 401 Unauthorized` | Refresh token expired | Re-run OAuth flow, update `SPOTIFY_REFRESH_TOKEN` |
| `spotify: 204 No Content` | No track playing | Open Spotify and play something |
| `config: SPOTIFY_CLIENT_ID is required` | `.env` not loaded | Check `.env` file exists and is populated |