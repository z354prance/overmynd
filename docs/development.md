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

To run Go checks in Docker instead of installing Go on the host:

```sh
docker run --rm -v "$PWD:/src" -w /src golang:1.24 \
  sh -c 'go mod verify && go vet ./... && go test -race ./...'
```

This mounts only the Overmynd checkout. A direct Go build in this root-run test
container may reject checkout ownership for VCS stamping; use `-buildvcs=false`
for that measurement. The normal image build excludes .git and needs no workaround.

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
now preserves supplied Trix/Judas/SubsPlease examples and labelled synthetic
edge cases in [the parser corpus](parser-fixtures.md). Confirmed absolute-numbering
mappings, duplicate acquisition and interrupted partial-placement acceptance tests
remain outstanding. Synthetic
names must be labelled synthetic; do not invent real examples or assume season 1.

## Phase gates

The compose smoke check passed locally on Linux with no host mounts or network;
the permanent representative fixture corpus remains incomplete. Phase 1A now has
commit-before-publish JSON behavior and a separately tested SQLite repository.
See [SQLite development](sqlite-development.md) for its dependency review and tests.
Error propagation through callers, migration/restore and importer reconciliation
remain required before its exit gate.
JSON import and SQLite backup/restore are separate explicit work; do not point a
development process at a changing production JSON file.
