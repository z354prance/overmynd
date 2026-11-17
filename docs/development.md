# Isolated Overmynd development

This is the first Phase 0 boundary, not a production deployment recipe.
Undermynd's repository, running container, settings, jobs, feeds, accounts and
service configuration are outside this work.

## Linux checks

From a clean Overmynd checkout with Go 1.24, run:

```sh
go vet ./...
go test -race ./...
docker build -t overmynd:development .
docker compose -f compose.development.yaml up --build -d
docker compose -f compose.development.yaml exec sandbox /overmynd healthcheck
docker compose -f compose.development.yaml down
```

The compose file has no host bind mounts, published ports, external network or
production image tags. Config and data use disposable tmpfs mounts owned by the unprivileged app user.
State disappears when the sandbox stops; restart durability is tested separately
with temporary disk-backed stores in the Go tests. No media library
is mounted; library inspection is therefore unavailable until a selected fixture
is explicitly mounted read-only. Do not copy production settings, jobs or keys
into this sandbox. The network-disabled sandbox is a startup/health smoke test,
not an interactive UI or live-adapter environment. It cannot fetch feeds or
submit remote downloads even if an address is accidentally entered.

Tests use temporary directories, fake backends and httptest services. They do
not require production credentials. Existing integration tests simulate remote
services; their success does not prove a real download or Tdarr conversion.
Do not run workflow_dispatch on main as a testing shortcut: it publishes images.

## Current interfaces

| Boundary | Current implementation | Rework direction |
| --- | --- | --- |
| Jobs | internal/store JSON snapshot | Error-aware repository, then SQLite |
| Acquisition | internal/backend, Engine.Override | Simulated backend by default in tests |
| Import confirmation | pipeline.ImportChecker, Engine.ArrFor | Mock arr; native verified assets later |
| Transcoding | internal/tdarr and pipeline/tdarr.go | Dedicated fixture input/output only |
| Intake | internal/rss and internal/watch | No production feeds or watched folders |
| Public/admin views | internal/web and internal/auth | Preserve sanitization and authenticated controls |

Legacy job IDs, protocol credentials, environment aliases and default paths are
compatibility contracts. The isolated recipe sets explicit paths and ports
instead of changing those contracts. The executable still supports live adapters;
this compose boundary does not claim a global simulation switch exists.

## Fixture coverage

See internal/library tests for parser cases and internal/pipeline tests for fake
download, transfer, transcode and placement flows. The roadmap's permanent corpus
still needs the user's actual Trix/Judas/SubsPlease names, absolute-numbering
mappings, batches, duplicate releases and interrupted partial placements. Synthetic
names must be labelled synthetic; do not invent real examples or assume season 1.

## Phase gates

Phase 0 is not complete until the compose smoke check and fixture isolation are
verified on Linux. Phase 1A starts with commit-before-publish behavior and a
repository boundary. SQLite selection/pinning, error propagation through all
callers, durable transitions/import intents, cancellation/concurrency/failure
coverage and restart recovery remain required before its exit gate.
JSON import and SQLite backup/restore are separate explicit work; do not point a
development process at a changing production JSON file.
