# CLAUDE.md — Beacon

> Autonomous briefing agent in Go. Monitors ArXiv + mood-aware delivery via Spotify genre analysis.
> This file is the single source of truth for all agents in this project.

---

## Project Overview

**Beacon** is a production-grade autonomous agent that:
1. Scrapes academic papers from ArXiv, Semantic Scholar, HuggingFace Papers, bioRxiv/medRxiv
2. Analyzes the user's current Spotify playback genres to detect mood
3. Generates briefings adapted to mood: summaries (gym/high-BPM) or full analysis (normal mood)
4. Delivers briefings via Telegram and/or email at 09:00 and 21:00 (Mérida, MX — America/Merida)

**Primary Goal:** Portfolio project for grad school applications in AI/ML.
**Stack:** Go 1.23+ · Claude API (claude-sonnet-4-20250514) · Spotify Web API · Telegram Bot API · Resend/SendGrid

---

## Repository Structure

```
beacon/
├── CLAUDE.md                    ← You are here
├── Makefile
├── go.mod
├── go.sum
├── .env.example
├── .github/
│   └── workflows/
│       ├── ci.yml
│       └── deploy.yml
├── .claude/
│   ├── agents/
│   │   ├── architect.md
│   │   ├── godev.md
│   │   ├── mleng.md
│   │   ├── security.md
│   │   ├── reviewer.md
│   │   ├── tester.md
│   │   └── docs.md
│   └── commands/
│       ├── test-mood.md
│       ├── dry-run.md
│       └── paper-fetch.md
├── cmd/
│   └── beacon/
│       └── main.go              ← Entry point
├── internal/
│   ├── config/
│   │   └── config.go            ← Env + validation
│   ├── scheduler/
│   │   └── scheduler.go         ← 9am/9pm cron logic
│   ├── mood/
│   │   ├── spotify.go           ← Spotify API client
│   │   ├── detector.go          ← genre → MoodLevel classification
│   │   └── types.go
│   ├── papers/
│   │   ├── fetcher.go           ← Multi-source orchestrator
│   │   ├── arxiv.go
│   │   ├── semantic_scholar.go
│   │   ├── huggingface.go
│   │   ├── biorxiv.go
│   │   └── types.go
│   ├── briefing/
│   │   ├── generator.go         ← Claude API calls
│   │   ├── formatter.go         ← Markdown/HTML rendering
│   │   └── types.go
│   ├── delivery/
│   │   ├── telegram.go
│   │   ├── email.go
│   │   └── types.go
│   └── store/
│       └── store.go             ← In-memory + optional SQLite for deferred 9am→9pm
└── tests/
    ├── integration/
    └── mocks/
```

---

## Core Domain Logic

### Mood Classification

```
MoodLevel = HIGH_BPM | NORMAL

HIGH_BPM  → genre contains any of [gym, workout, rap, hip-hop, trap, drill, corridos, corridos tumbados, sierreño, banda, reggaeton, dembow, latin trap, hard rock, metal, drum and bass, dnb, hardstyle, gabber]
NORMAL    → everything else (focus, lo-fi, jazz, classical, pop, etc.)
```

**If Spotify is not playing:** default to `NORMAL`.
**If Spotify API is unreachable:** default to `NORMAL`, log warning.

### Delivery Logic

```
┌─────────────┬───────────────────────────────────────────────────────────────┐
│  Time       │  Behavior                                                     │
├─────────────┼───────────────────────────────────────────────────────────────┤
│  09:00      │  Check mood                                                   │
│             │  HIGH_BPM → accumulate papers to store, DO NOT send          │
│             │  NORMAL   → generate + send full briefing (9am batch)        │
├─────────────┼───────────────────────────────────────────────────────────────┤
│  21:00      │  Check mood                                                   │
│             │  HIGH_BPM → pull stored 9am papers + new 9pm papers          │
│             │             → send SUMMARIES for all (double batch)          │
│             │  NORMAL   → pull stored 9am papers (if any) + new 9pm papers │
│             │             → send FULL papers with mini-summary header       │
└─────────────┴───────────────────────────────────────────────────────────────┘
```

### Briefing Format by Mood

**HIGH_BPM (any time):**
```
📄 [Title]
🏷️ [Authors · Source · Date]
⚡ [2–3 line summary — key finding only]

---
```

**NORMAL:**
```
📄 [Title]
🏷️ [Authors · Source · Date]

💡 TL;DR: [1 sentence — what this paper does and why it matters]

[Full structured analysis: motivation, method, results, implications, caveats]

🔗 [Link]

---
```

---

## Monitored Topics

| Topic | Sources |
|-------|---------|
| AI/ML (general) | ArXiv cs.AI, cs.LG, cs.CL |
| Healthcare AI | ArXiv cs.AI + q-bio, PubMed via Semantic Scholar |
| Brain-Computer Interfaces | ArXiv eess.SP, bioRxiv |
| Computer Vision | ArXiv cs.CV |
| Bioengineering / Biocomputing / Biomedicine | bioRxiv, medRxiv |
| Anthropic / Claude research | ArXiv cs.AI (author filter), HuggingFace Papers |

---

## Agent Roster

Each agent lives in `.claude/agents/<name>.md`. Agents are invoked by Claude Code.

| Agent | File | Responsibility |
|-------|------|---------------|
| Architect | `architect.md` | System design, package boundaries, interface contracts |
| Go Dev | `godev.md` | Implementation, idiomatic Go, error handling patterns |
| ML Engineer | `mleng.md` | Genre classifier, Claude prompt engineering, embedding logic |
| Security | `security.md` | Secrets handling, API key rotation, input sanitization |
| Reviewer | `reviewer.md` | Code review, performance, edge cases |
| Tester | `tester.md` | Unit + integration tests, mock generation |
| Docs | `docs.md` | Inline docs, README updates, changelog |

**Invocation pattern:**
```
Use the @godev agent to implement internal/mood/detector.go
Use the @tester agent to write tests for internal/scheduler/scheduler.go
```

---

## Custom Slash Commands

| Command | File | What it does |
|---------|------|-------------|
| `/test-mood` | `commands/test-mood.md` | Hits Spotify API and prints current MoodLevel to stdout |
| `/dry-run` | `commands/dry-run.md` | Full pipeline run — fetches papers, classifies, generates briefing, prints to terminal (no send) |
| `/paper-fetch` | `commands/paper-fetch.md` | Fetches papers from all sources for a given topic, prints raw output |

---

## Environment Variables

All secrets are loaded from `.env` (never committed). See `.env.example`.

```env
# Spotify
SPOTIFY_CLIENT_ID=
SPOTIFY_CLIENT_SECRET=
SPOTIFY_REFRESH_TOKEN=

# Anthropic
ANTHROPIC_API_KEY=

# Telegram
TELEGRAM_BOT_TOKEN=
TELEGRAM_CHAT_ID=

# Email (choose one)
RESEND_API_KEY=
SENDGRID_API_KEY=
DELIVERY_EMAIL_TO=

# App
TIMEZONE=America/Merida
LOG_LEVEL=info          # debug | info | warn | error
DRY_RUN=false           # true = print briefing, do not send
```

---

## Coding Standards

### Go Conventions

- **Error handling:** Always wrap with `fmt.Errorf("context: %w", err)`. Never discard errors silently.
- **Interfaces:** Define interfaces in the *consumer* package, not the provider.
- **Contexts:** All API calls must accept and propagate `context.Context`.
- **Logging:** Use `log/slog` with structured fields. No `fmt.Println` in production paths.
- **Configuration:** All config lives in `internal/config`. No magic globals.
- **Tests:** Table-driven tests. Mock external APIs with interfaces. Minimum 80% coverage on `internal/`.
- **Formatting:** `gofmt` + `goimports`. Enforced in CI.
- **Linting:** `golangci-lint` with `.golangci.yml`. No lint warnings merged to main.

### Claude API Usage

- **Model:** Always `claude-sonnet-4-20250514`. Never hardcode older versions.
- **Prompts:** Live in `internal/briefing/generator.go`. No prompt strings scattered across packages.
- **Max tokens:** 1500 for full analysis, 300 for summary mode.
- **Retries:** Exponential backoff, max 3 attempts, on 429 and 5xx.
- **Rate limiting:** Respect Anthropic limits. Process papers sequentially with 200ms delay between calls.

### Git Workflow

```
main          ← always deployable
└── feature/  ← all work branches
    fix/
    refactor/
```

- Commit format: `type(scope): description` — e.g. `feat(mood): add genre classifier`
- Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`
- PRs require: passing CI, no lint errors, test coverage maintained

---

## CI/CD (GitHub Actions)

**`ci.yml`** — runs on every PR:
1. `go vet ./...`
2. `golangci-lint run`
3. `go test ./... -race -coverprofile=coverage.out`
4. Coverage gate: fail if < 80%

**`deploy.yml`** — runs on merge to `main`:
1. Build binary
2. Deploy to target (VPS / Railway / Fly.io — TBD)
3. Smoke test: hit `/healthz` endpoint

---

## Makefile Targets

```makefile
make run          # go run cmd/beacon/main.go
make build        # go build -o bin/beacon cmd/beacon/main.go
make test         # go test ./... -race
make lint         # golangci-lint run
make dry-run      # DRY_RUN=true make run
make test-mood    # go run cmd/beacon/main.go --cmd=mood
make paper-fetch  # go run cmd/beacon/main.go --cmd=fetch --topic=AI
```

---

## Development Phases

### Week 1 — Foundation
- [ ] Repo init, `go.mod`, folder structure
- [ ] `internal/config` — load + validate all env vars
- [ ] `internal/mood/spotify.go` — OAuth refresh flow + now-playing endpoint
- [ ] `internal/mood/detector.go` — genre → MoodLevel
- [ ] `/test-mood` slash command working

### Week 2 — Paper Fetching
- [ ] `internal/papers/arxiv.go` — query by topic, parse XML feed
- [ ] `internal/papers/huggingface.go` — scrape Papers page
- [ ] `internal/papers/semantic_scholar.go` — REST API
- [ ] `internal/papers/biorxiv.go` — RSS feed
- [ ] `/paper-fetch` slash command working

### Week 3 — Briefing Generation
- [ ] `internal/briefing/generator.go` — Claude API integration
- [ ] Prompt engineering for HIGH_BPM (summary) vs NORMAL (full + TL;DR)
- [ ] `internal/store/store.go` — deferred 9am→9pm accumulation

### Week 4 — Delivery + Scheduler
- [ ] `internal/delivery/telegram.go`
- [ ] `internal/delivery/email.go` (Resend)
- [ ] `internal/scheduler/scheduler.go` — 9am/9pm cron with mood-aware branching
- [ ] `/dry-run` slash command working
- [ ] End-to-end test: full pipeline dry run

### Week 5 — Polish + Deploy
- [ ] CI/CD pipeline
- [ ] Integration tests with mocks
- [ ] README + architecture diagram
- [ ] Deploy to production

---

## Key Invariants

> These must never be violated. Any agent that would break an invariant must stop and ask.

1. **Never send at 9am if mood is HIGH_BPM.** Accumulate, never discard.
2. **Always include a TL;DR header on full papers.** Even in NORMAL mode, the user needs a 1-line preview.
3. **Spotify failure is non-fatal.** Default to NORMAL and continue.
4. **No secrets in logs.** API keys, tokens, and chat IDs must be redacted in all log output.
5. **DRY_RUN=true must never send real messages.** Any delivery function must check this flag first.
6. **Claude API calls are sequential.** Never concurrent Claude requests — respect rate limits.

---

## Contact / Context

- **Developer:** Working at AIVARA, Go stack, Grafana/Loki observability, GitHub Actions CI/CD
- **Goal:** Grad school application portfolio (AI/ML focus)
- **Location:** Mérida, Yucatán, MX (America/Merida timezone — all cron in local time)
- **Preferred language in agent responses:** Spanish