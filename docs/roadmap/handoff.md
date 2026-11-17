# Overmynd implementation handoff

Updated 2026-11-16. Read README.md in this directory for the active roadmap.

## What the user wants

Rework Overmynd using the copied Undermynd code as its baseline. These are separate
applications with separate containers. Undermynd remains in production and must
not be changed or repointed while Overmynd is developed. The eventual destination
is native acquisition/library management, starting with the anime RSS/import flow.
The user approved the controlled core rewrite and asked for the roadmap to be
reworked around it. Implementation is now authorized for Phase 0 and Phase 1A;
keep later phases behind their acceptance gates.

## Repository state

- Repository: https://github.com/z354prance/overmynd, branch main.
- Undermynd snapshot 0f46f7b was copied, then the project/module/executable renamed
  to Overmynd. PR #2 established this baseline and is merged.
- Existing adapters, UI and tests are inherited functional baseline code. The
  opt-in SQLite repository now exists; catalog/native importer work remains.
- OVERMYND_* environment aliases accept existing UNDERMYND_* names. Legacy client
  credentials and work-directory defaults were retained; do not casually rename
  paths or protocol IDs during development.
- Pushes run Linux CI. Container publication is manual workflow_dispatch on main.
  Do not publish a replacement image or touch either running container without
  an explicit deployment request.
- Original roadmaps, the review and active revised plan are approved for public
  GitHub publication. Preserve the source documents as historical references.

## Baseline code findings (before implementation)

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

## Implementation progress (2026-11-16)

Work branch: `rework/phase-0-1a-foundation`. Phase 0's disposable compose boundary
has passed a local Linux Docker build and healthcheck; no production resources
were mounted, and the sandbox was removed afterward. Public/admin baseline tests
continue passing. The permanent real-name fixture corpus remains outstanding.

The legacy repository interface, JSON commit-before-publish fix and slice clone
regression tests are committed. The executable still uses JSON. The separately
available context-aware SQLite repository has schema v1, WAL/FULL synchronization,
foreign keys, busy timeout, transactional job changes, full transition history and
per-file import intent records. See [SQLite development](../sqlite-development.md) for dependency review,
contracts and limits. modernc.org/sqlite v1.44.3 and matching libc v1.67.6 are pinned.

Local Docker validation: module verification, gofmt, vet, full Linux race suite,
CGO-disabled image build, XML parse and isolated compose healthcheck. Targeted
SQLite tests additionally prove SQLITE_FULL rollback, SQL failure rollback,
cancellation, independent concurrent writers, bounded-payload/unbounded-journal
history and copying-intent restart recovery. These are simulated fixture tests.

Follow-up: the opt-in `SQLite.ImportSnapshot` API now validates captured JSON bytes
and imports jobs, retained transitions and a SHA-256 migration marker atomically.
Tests cover restart/idempotent retry, payload/remote-ID preservation, invalid input,
SQL-failure rollback, cancellation, nonempty destinations and competing imports.
The original capture remains untouched; no backend submission or application
cutover occurs. Docker module verification, vet, Linux race tests and the
CGO-disabled image build were rerun. Local Go caches are excluded from Git and
Docker contexts. The inherited pipeline race package takes roughly two minutes
on this host; allow it to finish rather than assuming a hang.

The jobs-only backup/restore regression tests passed GitHub Linux CI (format,
vet, race tests, XML and Docker build). A development-only maintenance command now
provides explicit captured-snapshot import and jobs backup operations; see
[maintenance commands](../sqlite-maintenance.md). It never starts the pipeline or
changes the executable's storage selection. Complete operational capture/restore
is still outstanding.

Next: provide coordinated operational capture and restore,
and connect intents to safe import reconciliation. No SQLite application cutover,
real-service validation, production import or phase-complete claim has been made.
Do not start Phase 1B until the Phase 1A gate is evaluated.

## Continuation checkpoint (2026-11-16, PR #3)

Continue on `rework/phase-0-1a-foundation`, PR #3. Do not merge or deploy. The main
executable still opens JSON; `NewRepository` constructors are explicit opt-in
boundaries for tests and later cutover, not automatic storage selection.

Completed in this continuation:
- Correct SQLite Windows file URIs; storage/import/backup tests now run locally.
- Migrate pipeline, qBittorrent/SAB client APIs, public/admin views and progress
  tracking to the error-aware repository contract. Existing constructors adapt the
  JSON store using FromLegacy; never adapt SQLite errors into missing legacy jobs.
- Stop intake, cleanup, Tdarr capacity hand-off and loop progress on failed reads.
  Client APIs return failure rather than empty results or false successful deletes.
  Dashboard failures return sanitized 503 responses; progress retains its last good
  snapshot. The admin jobs stream reports temporary storage/update unavailability.
- Pass request/poller cancellation through intake, cleanup and relevant repository
  calls. Legacy JSON updates reject cancellation observed in their callback before
  save. JSON's filesystem save itself is blocking and cannot be recalled mid-write.
- Guard ignored job-state update results: a failed write stops the current stage
  (or a progress callback). Fix retry reset losing the current job on failed save.
- Keep newest-first queue ordering across JSON and SQLite; verify shared identity,
  clone isolation and failed-update rollback contracts.
- Add a permanent real-name/synthetic parser corpus with provenance and explicit
  unresolved numbering/alias/pack limitations. See ../parser-fixtures.md.

Validation: local Windows store, library and maintenance-command tests pass.
Linux cross-builds and affected package test binaries compile. GitHub Linux race,
format/vet/template and container checks have passed the earlier continuation
milestones; check the latest PR head before claiming the final head is green.
No real-service validation, SQL startup switch, production import or publication.

Important remaining gates before SQLite application startup:
1. Coordinated operational capture/restore including encrypted settings, keys,
   authentication/access state and other JSON files; jobs-only backup is not enough.
2. Bind import intents to verified no-replace filesystem operations and crash
   reconciliation. Current file movement and partial-placement completion semantics
   remain inherited behavior; this continuation does not certify them as safe.
3. Reconcile uncertain external effects. If a backend accepts Add but saving its
   remote ID fails, merely logging/stopping that stage cannot prove a later retry
   will not create another remote job. Add durable intent/reconciliation tests.
4. Resolve terminal-job resubmission semantics: JSON Put historically replaces a
   record; SQLite Put intentionally inserts only once. Explicit new attempts/requeue
   behavior must preserve external protocol IDs and transition history before
   callers can switch storage. Do not weaken SQLite Put into a blind upsert.
5. Review concurrency/leases and legacy constructor removal at the actual cutover.
   Read failure propagation alone is not proof of full application SQLite parity.
6. Evaluate Phase 0/1A gates before Phase 1B, catalog features or production changes.

The user asked to stop with 2% remaining in the shared five-hour account balance.
The Codex app get_usage_limits tool exposes a 300-minute window: remaining is
100 minus usedPercent. Check periodically; do not use reset credits automatically.
