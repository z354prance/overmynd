# Roadmap: folding the arr stack into Undermynd (revised Nov 16, 2026, after reading the repo)

Goal: one app that does what Radarr, Sonarr, Lidarr, Prowlarr, Seerr, Houndarr and Recyclarr do today, so a user never runs or configures a second service.

Revision history: v1 assumed a dashboard only. v2 used the build chat. v3 is based on the code in github.com/z354prance/Undermynd (about 12.6k lines of Go, standard library only, no third-party dependencies). v4 recorded the storage decision and linked the metadata deep-dive. v5 records that anime already runs without Sonarr and that the app is personal-only. v6 adds the anime sources. v7 records Emby as the media server, gap filling across release groups, and corrects the parser note. v8 records that Jellyfin is supported too. v9 records that nothing tells Emby to refresh today. v10 adds real Trix and Judas names, the parser results on them, per-release feeds and the 1080p preference.

## Decisions
- **Storage (decided Nov 16):** move everything to SQLite, including jobs, early. Use a pure-Go driver (no cgo) so the static binary and scratch image survive. Jobs and the new catalog share one database so jobs can reference titles with real foreign keys. This ends the "standard library only" rule; the only allowed dependency is the SQLite driver unless decided otherwise.
  - Migration safety: import the existing JSON job file once on first start and keep it as `jobs.json.bak`; keep the current store interface (`Get`, `List`, `Put`, `Update`) and run the existing tests against the new backend; WAL mode and a busy timeout; run a real TorBox + Tdarr grab on the new build before building on it.
- **Audience (decided Nov 16):** personal, non-commercial, self-hosted. TMDB's free terms apply; a Credits section with TMDB attribution is accepted. Each install uses its own TMDB key.
- **Anime (decided Nov 16):** first-class, and already native. The user does not use Sonarr for anime; the RSS/watch pipeline plus `internal/library` places it. Native anime metadata, monitoring and gap filling are in scope and come early.
- **Anime sources (Nov 16):** mostly SubsPlease, with per-show exceptions (Trix for Blue Box, Judas for The Student Council show, which has holes). nyaa.si is used through per-release RSS feeds. Preferred resolution is 1080p, globally. Details in `design/anime-pipeline.md`.
- **Media server (Nov 16):** Emby is the primary server and the one the user runs. Jellyfin is supported as well because it is close enough to Emby that little extra work is needed; the access flow already has separate Emby and Jellyfin paths. Plex is not in scope. Design new media-server features against one small interface (refresh library, list libraries, create/enable user, check sign-in) with an Emby and a Jellyfin implementation, and test on Emby first.
- **Library refresh (Nov 16):** nothing tells Emby to refresh today and Emby picks up new files on its own, so this is optional. Build it later as an off-by-default setting behind the media-server interface; check Emby's API for a targeted path update vs a full refresh first.

## Where Undermynd is today (from the repo)

Download and transcode layer (the original product):
- Fake qBittorrent API (port 8081) and fake SABnzbd API, so Sonarr, Radarr and Lidarr use it as their client.
- Backends: local qBittorrent, TorBox (usenet and torrents, rate limiter, resumable pull), NZBGet fallback.
- Job state machine with retries, a reconciler, cleanup, import confirmation through the arr's history API, and atomic moves into `complete/<category>`.
- Tdarr hand-off (scan, node/worker progress, stall detection, concurrency limit, per-category fail-or-keep-original).
- Checks: resolution mismatch against what the arr grabbed (blocklists fakes), container header and size validation.
- RSS feeds with regex rules, first-poll baseline, dedupe, skip reasons, manual download. The admin shows one feed per show with each item, "seen earlier", and a Download action. Watch folder for dropped .torrent/.nzb/.magnet files.
- Re-encode tab: converts files already in the library to AV1 through the same Tdarr path.
- The anime path, with no arr: `internal/library` parses release names (dash numbering, `S01E02`, `1x02`, dotted names, season suffixes like `2nd Season`, resolution), fuzzy-matches show folders, and `placeInLibrary` moves episodes into `<Show>/Season N/`. It refuses to guess. It has no episode list, no monitoring and no missing detection. Emby finds the placed files on its own.
- Tested on real names (Nov 16): Trix `SxxExx` and SubsPlease dash names parse and match. Judas names keep `{English title}` and `[Uncensored]` in the title, so the folder match refuses and the file waits for a person. Absolute numbers (`One Piece - 1158`) parse but the season is guessed as 1.

Merged from Overmynd (the README status table still says "not started", but the code is present):
- Public request box that talks to Seerr (search, discover, request, admin approve/decline). Seerr does the real work; Undermynd is a front end acting as one Seerr user.
- "Need access?" flow for Emby (Emby Connect) or Jellyfin: account request, admin approval, disabled copy of a template user, single-use setup link, email.
- Playback from Tracearr with Emby library filtering, recently added, storage monitor.

Platform:
- Admin with password + TOTP and recovery codes, AES-256-GCM secrets, public status page on 8080, Unraid template, CI to ghcr.io, scratch image about 7 MB.
- Persistence is a single JSON file written atomically (to be replaced, see Decisions).

What still lives in the arrs and Seerr (movies and TV; anime already does not):
- Indexer search and release scoring, quality profiles and custom formats, title metadata and monitoring, rename and import, upgrades, calendar, request approval logic, quotas, user accounts.

## What each tool does, and where it lands

| Tool | Job today | Undermynd home |
|---|---|---|
| Seerr | Discover, request, approve, quotas, status | Today: proxied. Target: native requests tied to pipeline stages |
| Prowlarr | Indexers (Torznab/Newznab), health, stats | Indexers module (new) |
| Radarr / Sonarr | Monitor, search, score, grab, import, rename, upgrade | Library, decision engine, importer (grows from `internal/library` and the pipeline) |
| Lidarr | Same loop for music, MusicBrainz, tagging | Music library (last) |
| Recyclarr | TRaSH Guides sync | Built-in guide profile importer |
| Houndarr | Rate-limited missing/cutoff/upgrade searches | Scheduler inside the engine (the RSS poller and arr `MissingItems` are the seed) |

## Principles
1. Strangler, which is how it is already built. Each phase replaces one arr responsibility and must work with that arr on or off.
2. Reverse the direction last. Today the arrs push grabs in. The end state is Undermynd choosing what to grab; the fake client APIs stay as compatibility mode.
3. Reuse community data: TRaSH Guides, Prowlarr-style indexer definitions, TMDB/MusicBrainz.
4. Every skipped or failed thing says why (already true for RSS). Make it true for every rejected release.
5. Be polite to indexers and metadata sources by default.
6. Keep dependencies few and deliberate. SQLite is the one big one.
7. Start where there is no arr to lean on. Anime already works without one, so it is the first place native metadata and monitoring prove themselves.

## Phases

### Phase 0: Foundations
Done: job engine, backends, Tdarr, admin, auth, secrets, storage monitor, packaging.
To do, in this order:
1. SQLite migration of jobs (see Decisions). No new features; behavior must not change. This is the first milestone.
2. Data model keyed by title, not download: titles, seasons/episodes, albums/tracks, files, releases, profiles, requests, users. Jobs reference titles.
3. Metadata layer with cache, per-source rate limiter and background refresh (TMDB first). See `design/metadata-sources.md`. Needed now because anime has no arr to read from.
4. Read-only sync from the existing arrs for movies and TV (extend `arr/library.go`: grabs, episodes, missing, imports) so each job knows what title it is for.
5. Settings screen and secrets beyond the current tabs. Update the README status table.
- Exit: a movie or TV job shows its title, season and episode without asking the arr each time.

### Phase 0.5: Native anime library (first native catalog, no arr)
Sources: SubsPlease first, with per-show exceptions. See `design/anime-pipeline.md`.
- Quick fix, before the catalog: make the parser strip `{...}` blocks and descriptive bracket tags such as `[Uncensored]` from titles (keeping the braces text as a synonym) so Judas releases match their folder. Add the real Trix, Judas and SubsPlease names above as tests. Check whether the Student Council holes are releases that were never in the feed, or ones left unplaced by this.
- Parser test corpus: grow it from the user's feeds, including all-digit CRC hashes and titles with numbers (`86 - Eighty Six - 02`, already handled), batches and `1920x1080`.
- Add anime series to the catalog from TMDB (and an anime source if the spike says TMDB is not enough): titles with alternate names and synonyms (for example `Ao no Hako` and `Blue Box`), seasons, episode counts and air dates.
- Replace fuzzy folder matching with title and synonym matching against the catalog; keep the "never guess, never overwrite" rule and show unmatched items, with the reason, for a person to resolve.
- Absolute numbering to season mapping is required: SubsPlease numbers some continuing shows absolutely, so `Title - 112` must land in the right season folder. Check how the user lays out long-running shows first.
- Monitoring with a per-show ordered group preference (for example Judas, then SubsPlease, then anything on nyaa) and 1080p as the global default. Undermynd generates the per-show nyaa feeds (group + title + resolution) that the user builds by hand today. Per-release feeds already avoid the global feed item limit.
- Gap filling: a catalog episode list shows holes per season; when a group's releases skip episodes, search only the missing episodes from the next group. Never re-download an episode that is already present from another group; replacement is a separate upgrade rule, off by default.
- Missing and complete tracking per season, shown with the existing season cells. Use air dates so "missing" does not fire before a release exists. Show group and resolution per episode.
- Skip transcoding when the source is already in the target codec (Trix releases are AV1); confirm what the current Tdarr flow does.
- Optional media-server refresh after a placement (off by default; Emby finds files on its own today).
- Exit: anime is managed from a catalog, not from folders and regex.
- Doing anime first also builds the parser, importer and monitoring pieces that Phase 3 reuses for movies and TV.

### Phase 1: Requests (Seerr parity)
Start: public request box already works against Seerr; access flow already native for Emby and Jellyfin.
- Native discover and search (uses the metadata layer), request model and approvals, quotas, auto-approve, per-user history.
- Users and sign-in against Emby first, with Jellyfin through the same media-server interface (the access flow already creates accounts on both).
- Notifications. Request status reads from pipeline stages.
- Still hands approved movie and TV requests to Sonarr/Radarr/Lidarr to add and monitor, until Phase 3. Anime requests can go straight to the native anime library. Needs ID mapping between TMDB and the arrs' TVDB IDs.
- Exit: Seerr can be turned off.

### Phase 2: Indexers (Prowlarr parity)
- Torznab and Newznab indexers, then a definition format for tracker sites (user supplied; ship no indexers). nyaa.si RSS is the first non-Torznab source to support, since anime depends on it.
- Test, health, backoff, per-indexer rate and grab limits, priority, tags.
- Unified search API used by the engine, interactive search and RSS.
- Expose to the arrs as a Torznab/Newznab proxy so they can use Undermynd for search before they are removed.
- Exit: Prowlarr can be turned off.

### Phase 3: Movies and TV engine (Radarr + Sonarr parity)
The bulk of the work. Seeds in the repo: the release name parser and folder matcher, `placeInLibrary`, arr history and missing calls, the pipeline itself, and the anime catalog from Phase 0.5.
- Library: add, monitor, root folders, existing-file scan, standard / daily types (anime already done).
- Release parser: extend `library.Parse` into a full parser with a large test corpus: quality, source, group, language, editions, multi-episode, season packs. It must map release numbering to catalog numbering with alternate titles.
- Decision engine: profile + custom formats + size limits + cutoff + upgrades + blocklist, with rejection reasons.
- Grab directly into the existing TorBox/client/Tdarr pipeline with no fake-client round trip.
- Import and rename: grow the placer into a full importer (hardlink/copy/move, naming schemes, season packs, extras, cleanup), keeping its "never guess, never overwrite" rule and adding a dry-run mode.
- Calendar, wanted/missing, cutoff unmet, history, manual search, manual import.
- Exit: Radarr and Sonarr can be turned off for a normal library.

### Phase 4: Profiles and guide sync (Recyclarr parity)
- Custom format engine ships in Phase 3; this adds the TRaSH importer: formats, groups, profiles with score sets, quality definitions, naming.
- Preview diff, local overrides, scheduled refresh, setup wizard.
- Undermynd-only extension: profiles can carry the transcode target (for example "AV1 after import") so quality policy and conversion policy live together.

### Phase 5: Search scheduler (Houndarr parity)
- Backlog sweeps for missing, cutoff-unmet and upgrades in small batches, per-item cooldown, post-release grace window, global indexer budget.
- Dashboard of what was searched and skipped, and why. Reuse the RSS skip-reason UI. Anime gap filling uses the same scheduler.

### Phase 6: Music (Lidarr parity)
Last. Artist/album/release model, MusicBrainz (1 request per second; local mirror or heavy caching), audio quality parsing, tagging, multi-disc. Reuses indexers, clients, profiles and scheduler.

### Phase 7: Switchover and polish
- Importers from existing Radarr, Sonarr, Lidarr, Prowlarr and Seerr.
- Backup and restore, API for third-party apps, webhooks.
- Keep the fake qBittorrent/SABnzbd APIs as an optional compatibility mode.

## Biggest risks
- Release parsing and matching (anime numbering, multi-episode, scene names). Anime is where the parser is exercised hardest. Group-specific title decorations (`{English title}`, `[Uncensored]`) already cause safe-but-unplaced files.
- Metadata: sources disagree on numbering, have licensing terms and rate limits, and the arrs hide a proxy layer. See `design/metadata-sources.md`. The anime source is not chosen yet.
- Mixed release groups in one season (quality, codec, audio differ). Show and track group per episode.
- Import and upgrade bugs can delete or duplicate files. Dry-run, hardlink-first, never overwrite.
- The SQLite migration touches the live download path; follow the safety steps under Decisions.
- Emby and Jellyfin are similar but not identical (sign-in, API details). Only Emby is tested by the user, so Jellyfin paths need their own tests or a clear "untested" label.
- Indexer terms and legal exposure: ship no indexers.
- Not yet proven against real services: the README says TorBox, Tdarr, NZBGet and the arr history check were tested against stand-ins (the build chat shows TorBox and Tdarr have since run for real). Do not stack the engine on top until the existing path is solid.
- Filesystem permissions across containers (already hit once with a container re-owning shared folders). The importer touches more paths; keep work and complete folders configurable.
- Scope: seven apps' worth of settings. Keep the UI opinionated, put the rest under Advanced.

## Suggested order
1. Parser fix for Judas-style titles plus real-name tests (small, immediate, helps the Student Council holes).
2. Phase 0 steps 1 and 2 (SQLite, data model).
3. Phase 0 step 3 (metadata layer) and the anime source spike.
4. Phase 0.5 (native anime library).
5. Phase 0 step 4, then Phases 1 and 2; neither needs the engine, both are visible wins.
6. Phase 3 movies first (simpler), then TV, then Phases 4 and 5.
7. Music after TV is stable.

## Open questions
- How are long-running shows laid out in the user's library (One Piece as season 1, season 23, or something else)?
- Are the Student Council holes releases missing from Judas's feed, or ones left unplaced by the title mismatch?
- Does the Tdarr flow skip AV1 sources like Trix's, or re-encode them?
- Which anime metadata source after the spike (TMDB alone, or AniList/AniDB/Kitsu in addition)?
- Single household or multi-user?
- Whisparr, Readarr/books (the storage monitor shows Books), subtitles (Bazarr): in scope or out? Assumed out.
- Is Overmynd retired now that its pieces are merged?

## References
- Metadata deep-dive: design/metadata-sources.md
- Anime pipeline: design/anime-pipeline.md
- Repo: https://github.com/z354prance/Undermynd
- Build chat: https://claude.ai/chat/0413172a-03f0-437f-b2f1-3c6cf7c7f59e
- Houndarr: https://av1155.github.io/houndarr/docs/concepts/how-houndarr-works
- Seerr merge: https://docs.seerr.dev/blog/seerr-release/
- Recyclarr features: https://next.recyclarr.dev/wiki/features/
- TRaSH Guides sync tools overview: https://docsearch.algolia.com/mcp/docs/repo/trash-guides/guides
