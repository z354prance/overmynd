# Overmynd implementation handoff

Updated 2026-11-16. Read README.md in this directory for the active roadmap.

## What the user wants

Rework Overmynd using the copied Undermynd code as its baseline. These are separate
applications with separate containers. Undermynd remains in production and must
not be changed or repointed while Overmynd is developed. The eventual destination
is native acquisition/library management, starting with the anime RSS/import flow.
The user approved the controlled core rewrite and asked for the roadmap to be
reworked around it. No feature implementation has been requested in this planning
step beyond preparing that plan.

## Repository state

- Repository: https://github.com/z354prance/overmynd, branch main.
- Undermynd snapshot 0f46f7b was copied, then the project/module/executable renamed
  to Overmynd. PR #2 established this baseline and is merged.
- Existing adapters, UI and tests are inherited functional baseline code. The
  redesigned SQLite/catalog/native importer does not exist yet.
- OVERMYND_* environment aliases accept existing UNDERMYND_* names. Legacy client
  credentials and work-directory defaults were retained; do not casually rename
  paths or protocol IDs during development.
- Pushes run Linux CI. Container publication is manual workflow_dispatch on main.
  Do not publish a replacement image or touch either running container without
  an explicit deployment request.
- Original roadmaps, the review and active revised plan are approved for public
  GitHub publication. Preserve the source documents as historical references.

## Confirmed code findings

- internal/store/store.go: JSON in-memory store writes the whole file; updates
  mutate memory before save success; Get/List cannot report storage read errors.
- internal/pipeline/library.go: unmatched/partial placements become Cleaned.
- internal/pipeline/engine.go moveFile: rename/copy can overwrite after a separate
  destination check; cross-filesystem copy writes the final path directly.
- internal/rss/rss.go: seen is marked before applying rules; outcomes are in memory.
- internal/library/library.go: Judas alias/tag decorations remain in Title;
  absolute numbering defaults to season 1 with Marked=false. Real-name probes
  reproduced these behaviors. Their temporary test file was removed.
- internal/pipeline/quality.go: RSS/watch jobs skip the arr-derived quality check.
- internal/pipeline/tdarr.go: ordinary hand-off has no AV1 precheck; re-encode code
  inspects codec but that does not prove all AV1 input bypasses video processing.

## First work and constraints

Follow Phase 0 and Phase 1A, not a big-bang rewrite. Separate test paths/ports and
owned outputs; mock remote services by default. Add SQLite with a pure-Go driver,
explicit storage errors, migrations and restart/restore tests. Preserve protocol
IDs and encrypted secrets. Transactions cannot make filesystem moves atomic:
import intents and reconciliation are required. Unknown episode numbering must
remain reviewable. Do not let both apps own one production acquisition.

The initial anime milestone uses existing RSS feeds/rules and manually confirmed
show/episode mappings. Metadata/search automation follows the reliable workflow.
No need to demand all open roadmap decisions before foundational work begins.

## Validation and communication

Linux CI runs gofmt, vet, race tests, XML validation and Docker build. The inherited
pipeline ownership code uses syscall.Stat_t and prevents full native Windows tests;
a Linux environment is required. Before declaring a phase complete, run its actual
acceptance tests and distinguish simulated checks from real-service validation.

Work in small commits. The user wants direct progress and concise plain-language
updates. Keep production Undermynd separate. Do not interpret old requests from the
original dashboard conversation as a current instruction to redeploy Overmynd.
