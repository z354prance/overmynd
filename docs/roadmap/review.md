# Overmynd roadmap review

Review date: 2026-11-16. Reviewed against the Undermynd snapshot 0f46f7b now copied
and renamed into Overmynd main. The original supplied roadmaps are preserved beside
this review. This document recommends changes; it does not implement the overhaul.
Undermynd remains a separate working application and container. Overmynd is the
replacement under development, with automatic image publication held.

## Recommendation

Keep the modular monolith, the backend integrations, Tdarr hand-off, administrator
security and existing compatibility APIs. Rework persistence, content identity and
importing before replacing the arr applications. Anime is a good first vertical
slice, but it needs a small search adapter and durable import review earlier than
the supplied phase ordering suggests. Avoid a simultaneous rewrite of pipeline,
database, UI and metadata.

## What the code confirms, and where the roadmap needs correction

| Finding | Code | Recommended adjustment |
|---|---|---|
| The store is a concrete in-memory JSON store. Engine depends on `*store.Store`, not an interchangeable storage interface. Put/Update change memory before persistence succeeds. | `internal/store/store.go`, `internal/pipeline/engine.go` | Introduce a repository boundary and enforce commit-before-publish state changes. Preserve semantics deliberately, rather than swapping implementations under signatures that cannot return database errors. |
| Other operational state is also persisted separately: RSS seen state, limiter state, admin/session files, access requests, settings and progress tracking. | `internal/rss`, `internal/backend/limiter.go`, `internal/auth`, `internal/access`, `internal/config`, `internal/progress` | Inventory each file and migrate in controlled stages. Jobs first is sensible; do not claim that moving jobs alone moves everything into SQLite. |
| Unmatched library files are preserved but the job is marked Cleaned, including partial placements. Cleaned jobs can be removed from persistence after seven days. | `internal/pipeline/library.go:21`, `internal/store/store.go:45` | Add durable Needs Review/Partially Imported outcomes and per-file records. Cleanup of backend artifacts is distinct from successful library import. |
| The placer checks destination existence before calling moveFile. Same-filesystem rename can replace a destination created after that check. Cross-filesystem copying opens the final destination with truncation and exposes partial content. | `internal/pipeline/library.go:78`, `internal/pipeline/engine.go:1056` | The current never-overwrite guarantee is incomplete. Add reserved destinations, temporary files, verification and no-replace finalization before expanding automatic imports. |
| Filesystem permission handling deliberately broadens files/directories to 0666/0777. | `internal/pipeline/perms.go`, `internal/pipeline/library.go` | Make ownership/mode policy explicit and limit it to files/directories created for a job. Do not recursively change pre-existing library permissions. |
| Judas decorations remain in the parsed title; absolute numbering produces an unmarked season 1. | `internal/library/library.go:40`, confirmed with supplied release names | Preserve raw title, aliases, release group and numbering evidence. A missing season is unknown/absolute, not proof of season 1. Require a mapping or an explicit per-show policy for placement. |
| RSS marks an item seen before applying a rule, and detailed outcomes are only in memory. | `internal/rss/rss.go:237` | Store ingestion, decision, reservation and submission separately. A transient submission failure needs a durable retry state, not just seen earlier. |
| Resolution checking explicitly skips RSS/watch jobs and relies on an arr for the expected quality. | `internal/pipeline/quality.go:24` | Native jobs need expected quality/profile data of their own. Verify actual files against that expectation without requiring an arr. |
| Normal Tdarr hand-off sends video files without a codec pre-check. AV1 checks exist in re-encode handling. | `internal/pipeline/tdarr.go:28`, `internal/pipeline/reencode.go` | Do not assume Overmynd skips AV1 today. Inspect the actual file and evaluate a conversion policy; AV1 can skip video encoding while still needing audio, subtitle or container work. |
| Requests currently delegate to Seerr; media access provisioning and administrator login are separate systems. | `internal/web/requests.go`, `internal/access`, `internal/auth` | Build native requester identity and authorization explicitly. A media-server login must not grant Overmynd administrator permissions. |

## Revised implementation order and exit gates

### A. Baseline and import safety

Keep the current pipeline operational as the reference behavior. Capture test
fixtures from representative TorBox torrent/Usenet, local qBittorrent, NZBGet,
Tdarr, RSS and manual-placement cases. Use a dedicated Overmynd configuration,
work directory and machine port during development. Undermynd owns live downloads.
Read the existing media library without writing during catalog exploration.

Fix no-replace import finalization, path/symlink containment, durable unresolved
items and error propagation before widening automatic placement. Include disk full,
permission failure, process restart, concurrent imports and cross-filesystem copies.
Restore missing/successful/failed diagnostics in the UI instead of flattening these
outcomes into Cleaned. The Judas parser correction fits here as a small tested fix,
but is not a complete solution to gap filling.

Exit: the same episode cannot be silently overwritten; an unresolved file remains
visible across restarts and cleanup. No permissions outside owned output change.

### B. SQLite without a behavior rewrite

Use the agreed pure-Go SQLite driver, pinned with audited transitive dependencies.
Treat the decision as one selected storage driver, not a promise of literally one
module in the dependency graph. Measure the new binary size; the existing 7 MB
claim will need revalidation.

Start with a stable job record/payload plus queryable state and an append-only
transition table. Add nullable catalog references so legacy and manual jobs can
exist before metadata resolution. Avoid creating the entire music schema now.

Provide Get/List failures separately from not found: the current boolean-only Get
must not interpret a database read failure as permission to create another grab.
Use short transactions and explicit job version checks. Apply WAL, foreign keys,
busy timeout and connection settings consistently; keep the database on local
appdata storage. Do not hold a transaction during network calls or file copies.

Import a validated JSON snapshot in one transaction, record a migration marker,
verify identifiers/counts/state, then retain the untouched original plus backup.
A restart must not reimport it or create duplicate remote jobs. Include all queued,
failed, partial-placement and cleanup state. Migrate RSS receipts/reservations next,
then the remaining operational files in named steps; retain secret.key and encrypted
secrets throughout. Backups must be SQLite-consistent and include the encryption key.

SQLite and the filesystem cannot commit together. Record import intent and results,
and reconcile interrupted file operations instead of claiming a single database
transaction makes file movement atomic.

Exit: existing state-machine tests pass; injected write failures do not publish
uncommitted state; interrupted migration resumes once; a restore test passes. A
controlled Overmynd-only real grab is the live-validation gate, never an automatic
change to Undermynd.

### C. Identity, inventory and metadata spike

Add titles, external IDs, aliases, episodes, numbering mappings, releases, assets,
asset-to-episode associations and job associations. One job may cover many episodes;
one episode may have multiple files or release versions. Keep external client
hash/NZB IDs distinct from internal catalog IDs. Preserve existing identifiers used
by the compatibility APIs.

Use provider-qualified IDs and stable internal IDs. Store original numbering,
seasonal numbering and absolute numbering separately, with mapping provenance and
confidence. A synonym is a candidate for a title, not proof of season/episode mapping.
Movies and television need different metadata adapters even if they share caching.

Test a small anime sample before selecting the mapping source: Blue Box S2,
Student Council, One Piece, My Hero Academia, split cours, specials, renamed shows,
combined episodes and batches. Require demonstrated coverage of numbering and Emby
file recognition; TMDB title discovery alone is not enough evidence. The referenced
`design/metadata-sources.md` was not supplied and is absent from this baseline, so
that decision is still open.

Import an existing file inventory read-only first. Unknown metadata, unknown air
dates, airing episodes, delayed releases, queued jobs, unmatched files and genuine
missing episodes must have separate outcomes. Availability comes from validated
assets plus active reservations, not from whether a feed has been seen. Reuse the
previous missing-episode fixes as tests: future/TBA episodes must not keep shows
in progress forever.

Exit: representative existing files map without moving or renaming them; ambiguous
items remain unresolved with actionable explanations; provider outages do not
change an episode from available to missing.

### D. Native anime slice with minimal search and scheduling

Move a narrow indexer-search client and a shared budgeted task scheduler forward
from the later phases. RSS alone cannot recover an older episode outside a feed's
window. Use the existing Prowlarr service as an optional bridge, or the selected
anime source adapter, while native indexer management is being built. Replacing
Prowlarr entirely is a later milestone, not a prerequisite for native anime.

Add per-show aliases/numbering policy, ordered release-group preferences, 1080p
default, language/audio preferences, monitored episodes and an inventory view.
Identify the episode before comparing groups. Reserve missing episodes atomically
before submission, recheck at import, and deduplicate cross-feed/cross-group grabs.
Mixed groups are allowed; upgrades remain disabled by default. Distinguish unknown
numbering from a rejected release, and never strip every bracketed phrase blindly:
keep raw names, extract useful tags and normalize only recognized decorations.

Include release revisions, multi-episode files and batch coverage in the model now;
ambiguous packs can require review until selection is implemented. A missing
preferred-group release should produce an explanation and a controlled fallback,
not an endless search loop. Rate limits, backoff, cooldown and a global task budget
are required now, not deferred until Houndarr parity.

Exit: manage one show end to end without Sonarr, fill a known gap from a second
group, restart during a grab, and avoid downloading any already-owned episode.
Verify the resulting filenames against Emby before automatic long-running anime
placement. Optional media-server refresh remains off by default and later work.

### E. Requests, movie-first engine and gradual retirement

Build native search/discover/request state once catalog identity and authorization
exist. Keep requester accounts, media-server accounts and administrator roles
separate; retain optional guest requests as a deliberate policy rather than an
accidental shared identity. Preserve pending Seerr requests before turning Seerr off.

For each managed title select one owner: external arr or native Overmynd. Both may
coexist across different titles, but cannot compete to import, upgrade or clean the
same title. Explicitly switch authority only after inventory and mappings reconcile.

Bring the core quality/profile/rejection model into the native anime/movie work.
Guide sync and its preview/override behavior can stay later; required profile policy
cannot wait until all movies and television are replaced. Movies first, then TV
season packs/daily episodes, then music. Full tracker-definition frameworks and
Torznab proxy compatibility should be separate optional work rather than gates for
the basic engine.

Every retirement is independent: switch off Seerr, Prowlarr, Radarr, Sonarr and Lidarr
only after each capability passes its own normal-use and restart/restore checks.
Keep compatibility client APIs available until the final external owner is gone.

## Architecture boundaries

Keep one deployable Go service. Suggested boundaries: persistence/repositories,
catalog, metadata adapters, release parsing, inventory, decision policy, task
scheduler, acquisition backends, processing policy, importer, requests and web views.
Do not introduce microservices or a plugin engine to achieve separation.

Use a small media-server interface with capability-specific behavior. Sharing Emby
and Jellyfin transport code is useful, but Connect linking and ordinary Jellyfin
login are different capabilities; test them independently. Keep Tracearr while it
provides reliable playback/recent additions; replacing it is not part of the stated
arr-consolidation goal.

A full UI overhaul should follow this domain model: Library, Wanted, Downloads,
Needs Review, Requests and Settings, with a public status view built from a sanitized
projection. Drive long-term recently-added and series progress from the catalog and
confirmed import events, not seven-day job retention or display-name grouping.

## Questions needed before implementation choices

- Actual Emby-compatible numbering/file examples for One Piece and another
  long-running show. Do not adopt the document's Plex season example as Emby truth.
- Exact Judas episodes missing and whether corresponding files/jobs are currently
  unplaced. Diagnose these before scheduling replacements.
- Current Tdarr flow's required audio/subtitle/container work, not only its AV1 target.
- Guest requests versus individually authenticated requesters and approval policy.
- Whether books/adult media/subtitle automation remain out of scope. Default to the
  roadmap's stated scope until explicitly expanded.

## Suggested first deliverable

An Overmynd development build containing reliable import review and no-replace
operations, parser regression tests using the real names, then SQLite jobs plus
migration/restore tests. No new download source, catalog automation or UI redesign
in that first milestone. This provides a measured base for the larger overhaul.
