# Overmynd rework roadmap

Updated 2026-11-16. This is the active implementation plan. It supersedes the
phase ordering in the two source roadmaps and the earlier review, which remain
as historical references. Status: Phase 0 and Phase 1A foundations implemented; exit gates remain open.

## Product and deployment boundary

Undermynd and Overmynd are separate applications and Docker containers. Undermynd
continues doing production work. Overmynd main contains the copied application as
a development baseline; we are reworking Overmynd into its eventual replacement.
Do not modify Undermynd, repoint its integrations, or operate against its job/config
files. Overmynd push CI builds/tests but does not publish images automatically.
A development build is not authorization for production switchover.

The long-term target is one personal, self-hosted app covering native requests,
metadata, monitoring, indexer search, release decisions, downloading, conversion,
import, history and scheduling. Movies, television, anime and eventually music are
in scope. Emby is primary; Jellyfin needs separate adapter tests. Plex, books,
adult-media automation and subtitle-service replacement are not currently scoped.

Keep a modular Go monolith and useful existing adapters, authentication and tests.
Rewrite core persistence, identity and import workflow in small increments.
SQLite is the agreed new dependency: select and pin a pure-Go driver and review
its transitive dependencies. Additional runtime tools/dependencies need an explicit
decision. Re-measure binary size and resource use after the storage change.

## First complete workflow

RSS -> identify episode -> inspect owned files and active reservations -> decide
-> download -> inspect/optionally process through Tdarr -> safe import -> confirm
availability -> cleanup.

Use existing feeds and rules. A small manually confirmed anime catalog comes before
automatic metadata discovery. The first workflow must not depend on Sonarr, Seerr,
a new indexer framework or an absolute-numbering guess. Initially, ambiguous
numbering and batches can go to Needs Review. Each phase below is independently
reviewable; none requires switching production off Undermynd.

## Phase 0: Establish a testable development boundary

- Separate Overmynd config, database, work and complete directories, ports, client
  categories and dedicated Tdarr input/output folders. Never share a writable job
  store or let both applications manage one download.
- Default to simulated adapters and read-only library inspection. Live Overmynd
  tests use explicitly selected fixtures and isolated output folders. Production
  library write access is a later, deliberate gate.
- Document the current interfaces and build a representative fixture set: ordinary
  SubsPlease episodes, Trix seasonal names, Judas aliases/tags, absolute numbering,
  batches, duplicate releases, failed transfers and partial imports.
- Establish a repeatable Linux test environment. Keep Linux ownership code behind
  appropriate boundaries so storage/parser/unit work can also run locally where
  practical. Keep public views sanitized and administrator controls authenticated.

Exit: Overmynd can run and be tested without consuming Undermynd's feeds/jobs or
changing its files, accounts or service configuration.

## Phase 1: SQLite and durable workflow records

### 1A. Jobs, transitions and import intents

- Introduce a repository interface; replace concrete `*store.Store` coupling.
  Reads distinguish missing records from database failures. Publish a state change
  only after its transaction commits; preserve the existing job IDs used by client
  APIs while introducing separate stable catalog identities.
- Start with queryable job state plus a compatible job payload and transition
  history, rather than normalizing every future feature in the first migration.
  Catalog references are nullable for manual and legacy jobs.
- Use short transactions, WAL, foreign keys, a busy timeout and consistent
  connection configuration. Database lives on local appdata, not a media share.
  No network or file-copy operations inside write transactions.
- Add durable per-file import records and intents: identified, planned, copying,
  finalized, verified, needs_review, conflict and failed. Job cleanup and content
  availability are separate facts. Partial imports do not become successful
  simply because the backend was cleaned up.
- Test disk-full/write failures, competing updates, cancelled work and restart
  recovery. State shown to callers cannot get ahead of committed state.

### 1B. RSS receipts and acquisition reservations

- Persist feed entries, first-check baselines, decisions, retry eligibility and
  job associations. An observed feed entry is not the same as a submitted job.
- Commit an episode reservation before acquisition. Deduplicate feed identity,
  release identity and episode coverage at their respective boundaries.
- Retain durable outcomes for skipped, queued, rejected and failed entries.
  Preserve per-source cooldown and limiter state across restarts.

### 1C. Import/restore tooling

- Validate a captured JSON snapshot, import it transactionally once, record a
  migration marker and verify identifiers/counts/state before declaring success.
  Preserve the original and a backup. Restarting migration must not submit another
  remote download. Do not import a changing production JSON file.
- Migrate settings, access requests and other operational state in named follow-up
  steps. Keep encrypted secrets and their key together; do not combine password,
  account or encryption redesign with the jobs migration.
- Provide a consistent database backup/restore path including secret.key and
  schema version. A JSON database copy or a bare live SQLite file copy is not the
  backup procedure. Prove restore against the development instance.

Exit: current job behavior survives the storage change; RSS retry decisions and
reservations survive restart; migration and restore tests pass. No duplicate
submission after a failed local commit or an uncertain remote response.

## Phase 2: Minimal anime identity and reliable importing

This is the first end-to-end milestone, not the full metadata product.

### Identity and parser

- Add a small catalog: shows, aliases, episodes, numbering mappings, releases,
  assets and asset-to-episode/job relationships. One download may cover several
  episodes; an episode can have multiple assets. Availability is derived from
  verified assets, not a display name or a torrent hash.
- Manually add/confirm the first shows, library folders, expected episode mappings,
  preferred groups and profile. Existing RSS rules remain usable.
- Preserve original release names and evidence. Extract recognized metadata tags
  and aliases (including Judas braces) without deleting arbitrary bracketed title
  text. Record group, resolution, revision and numbering mode.
- Missing season information means unknown or absolute, never an automatic season
  1 decision. A user-supplied per-show mapping can enable an absolute-numbered show
  before automated provider mappings exist. Unknown mappings remain reviewable.
- Use the real Trix/Judas/SubsPlease examples as permanent regression fixtures.
  Add numbered titles, all-digit hashes, multi-episode and batch cases. Require
  review when a pack's exact episode coverage cannot be established.

### Inventory and duplicate prevention

- Scan selected existing files read-only and map them to catalog episodes, with
  confidence and manual confirmation. Do not rename an entire existing library.
- Track verified available, actively reserved, processing, awaiting import,
  needs review, expected/unreleased and genuinely missing as distinct outcomes.
- Default to 1080p and per-show ordered group preference. Already-owned episodes
  block another acquisition across groups. Upgrades are a separate explicit policy,
  disabled by default. Recheck ownership just before final import.

### Safe importer

- Build an import plan with source/destination, episode mapping and conflict
  reasons. Show dry-run results. Prevent path/symlink escapes and reserve targets.
- Never overwrite an existing destination for an ordinary import. Implement
  no-replace finalization, not a check-then-rename promise. For cross-filesystem
  transfers, copy to a unique temporary file, verify and flush it, then finalize
  without replacing another file. Keep the source until success is recorded.
- Journal intent and reconcile interruptions: SQLite cannot atomically commit a
  filesystem move. Verify an existing destination before accepting it as a prior
  attempt's result. Make retries idempotent.
- Apply configured permissions only to owned new outputs. Preserve sidecars and
  existing folders. A partially imported release remains Needs Review; cleanup
  cannot erase unresolved files or their review record.

### Processing policy

- Reuse transfer and Tdarr adapters behind the new workflow. Inspect actual files;
  filenames are hints, not codec/quality verification.
- AV1 can skip video re-encoding when policy is satisfied. Still evaluate audio,
  subtitles, container and integrity requirements. Uncertain inspection produces
  a documented fallback rather than silent acceptance.
- Native jobs carry expected profile/quality themselves; existing arr-only quality
  checks cannot protect RSS/native acquisitions. Document current validation limits;
  choose any stronger inspection tool separately under the dependency policy.

Exit: one normal show and one difficult mapped show complete the entire RSS-to-
library path. Two groups releasing one episode yield one acquisition. Restart
before/after finalization, partial placement, conflict and failed conversion leave
recoverable state. Emby recognizes the selected filename/numbering convention.

## Phase 3: Metadata-assisted anime and gap filling

- Run the metadata spike before selecting a provider combination. The source
  documents' referenced metadata-sources.md has not been supplied. Test TMDB and
  candidate anime mappings against split cours, specials, long-running absolute
  numbering, aliases, combined episodes and the user's actual Emby layout.
- Store provider-qualified external IDs, mapping provenance and confidence under
  stable internal IDs. Discovery identity and episode numbering are separate.
  Cache responses, rate-limit by provider, retain last-good data and support manual
  corrections. Preserve corrections across metadata refreshes.
- Replace manual episode entry gradually; retain manual mappings as an override.
  Unknown/TBA dates and still-airing episodes cannot be labelled missing. Add a
  configurable release grace period without inventing a release date.
- Add a narrow search adapter for older releases outside RSS windows. Existing
  Prowlarr can serve as a temporary bridge. Full indexer management is not required
  to prove native anime search.
- Build the shared budgeted scheduler here: per-source backoff, per-episode cooldown,
  active-job reservations and explainable decisions. Reuse it for later backlog work.
- Fill only verified gaps, following group preference and compatible profiles.
  Distinguish unavailable preferred releases from transient failures; permit
  controlled fallback. Generate per-show feeds as a convenience after identity is
  reliable. Keep existing manual feeds/rules.
- Optional targeted Emby/Jellyfin refresh remains off by default. Resolve provider
  API behavior and test targeted updates before enabling it.

Exit: fill a known missing episode from a fallback group without reacquiring owned
content, flooding an indexer or replacing a user's file. Provider outages do not
make imported episodes become missing.

## Phase 4: Native requests and movie-first expansion

- Introduce requester identity, approvals, quotas, notifications and history from
  persisted catalog/pipeline facts. Media-server accounts and administrator roles
  remain separate; do not equate successful Emby login with admin access.
- Decide guest requests versus individually authenticated requests explicitly.
  Keep existing Seerr integration available until native requests cover the actual
  use cases and pending requests have been reconciled.
- Add movie metadata, inventory and native import using the same importer and
  scheduler. Bring quality profiles, rejection reasons, size/language/source rules
  and blocklists into this milestone. TRaSH import is later; basic policy is needed now.
- Per-title management owner is external arr or native Overmynd. Both may coexist
  across titles but never compete to import/upgrade/clean the same title.
- Compatibility qBittorrent/SAB APIs remain adapters, not a required round trip for
  native acquisitions.

Exit: native movie requests work through approval, acquisition and confirmed import.
A controlled group of titles can be managed without Radarr/Seerr, with rollback and
no competing owner. This is not yet global service retirement.

## Phase 5: TV engine, indexer management and profile synchronization

- Extend TV handling for seasonal/daily numbering, season packs, multi-episode
  files, upgrades, manual import, calendar and wanted/cutoff views. Preserve anime
  overrides and mapping evidence rather than forcing one numbering system.
- Add first-class Torznab/Newznab indexer configuration, health, rate/grab limits,
  tags, priority and interactive search. Advanced tracker definitions and an
  arr-facing proxy are separately scoped compatibility work.
- Expand the existing scheduler to backlog/cutoff/upgrade searches under one budget.
  Add guide sync with preview diff, local overrides and versioned profile changes.
- Keep source acquisition quality separate from the encoded library asset's format:
  conversion to AV1 must not itself trigger an endless quality-upgrade loop.

Exit: representative movie, regular TV and anime libraries work with clear decisions
and restart/restore behavior. Retire external services one responsibility at a time
only after the corresponding acceptance tests and actual use are satisfactory.

## Phase 6: Music and remaining service retirement

- Music comes after stable TV: artist/album/release/track identity, metadata mappings,
  tagging, multi-disc handling and audio profiles. Reuse inventory, jobs, scheduler
  and safe import operations without forcing music into an episode schema.
- Add migration/import tools for the remaining arr/indexer/request data. Preserve
  external IDs, profile intent and history where needed for decisions.
- Keep external-client compatibility optional for integrations that still need it.

Exit: retirement gates are recorded separately for Seerr, Prowlarr, Radarr, Sonarr,
Lidarr, Recyclarr and Houndarr. A checklist of actual needed capabilities replaces
an unrealistic claim to implement every feature of all seven projects at once.

## UI and architecture throughout

Organize around Library, Wanted, Downloads, Needs Review, Requests and Settings.
Keep the public status view and private administrative controls. Reuse adapters
and domain projections; prevent business rules from living in browser render code.
Do not do a speculative full visual redesign before the first native workflow works.
Playback/recent additions can continue using Tracearr; replacing it is not a goal.

Suggested boundaries: repositories, catalog/inventory, parser/mapping, metadata,
policy, scheduler, acquisition, processing, importer, request/access services, web.
One executable/container; no microservice or arbitrary plugin framework prerequisite.
Keep active job retention separate from persistent library/import history.

## Decisions and milestones requiring real examples

- Confirm the user's Emby filename/season mapping for One Piece and another
  absolute-numbered series before enabling those imports.
- Diagnose the specific Student Council gaps: feed absence, parse failure,
  processing failure and unplaced files are different causes.
- Inspect Tdarr policy requirements before enabling AV1 bypass decisions.
- Confirm requester identity/approval policy before replacing Seerr.
- Choose metadata providers and the pinned SQLite driver during their spikes.

These do not block foundational storage/import work. Undermynd keeps production
running while Overmynd reaches each exit gate. No dates are promised before the
first end-to-end milestone provides a basis for estimating the rest.

## Starting implementation scope

Start with Phase 0 and Phase 1A. Deliver separate reviewable commits for repository
boundaries, SQLite transactions and migration/restore tests. Then add RSS receipt
persistence and the minimal anime workflow. Stop at each exit gate to evaluate
behavior before expanding scope. Do not begin the entire roadmap in one change.

## References

- [Implementation handoff](handoff.md)
- [Earlier code-based review](review.md)
- [Original consolidation roadmap](source-arr-consolidation.md)
- [Original anime pipeline roadmap](source-anime-pipeline.md)
