# Anime pipeline: what we know and what it means (Nov 16, 2026)

Anime does not go through Sonarr. It arrives through Undermynd's RSS poller and watch folder, and `internal/library` places it. The media server is Emby (Jellyfin also supported). This doc captures the sources and what they imply for the native anime library (roadmap Phase 0.5).

## How the user gets anime
- Primary: SubsPlease.
- Exceptions per show: Blue Box from Trix (Season 2); Seitokai ni mo Ana wa Aru (Even the Student Council Has Its Holes) from Judas, which has holes to fill from another group.
- When using nyaa.si the user sets up an RSS feed for that specific release, not a global feed. The admin feeds list shows one feed per show ("Blue Box", "Seitokai ni mo ana wa aru") with every item, "seen earlier", and a Download action.
- Preferred resolution: 1080p, globally.

## Real names (from the user's feeds) and what the parser does
Tested against `library.Parse` and `library.Match` in the repo on Nov 16 (a throwaway test, removed afterwards):

| Release name | Parsed title | Season/episode | Match to a folder named as below |
|---|---|---|---|
| `[Trix] Blue Box S02E06 1080p NF WEB-DL Opus2.0 AV1 (Multi-Audio / Dual-Audio, Multi-Subs) (Ao no Hako Season 2) (Weekly)` | `Blue Box` | S2 E6, 1080p | matches `Blue Box` |
| `[Judas] Seitokai ni mo Ana wa Aru! {Even the Student Council Has Its Holes} [Uncensored] - S01E07 [1080p][HEVC x265 10bit][Multi-Subs] (Weekly)` | `Seitokai ni mo Ana wa Aru! {Even the Student Council Has Its Holes} [Uncensored]` | S1 E7, 1080p | no match; the closest folder `Seitokai ni mo Ana wa Aru` is only offered as a candidate, so the episode is left for a person |
| `[SubsPlease] Blue Box - 06 (1080p) [48F1910E].mkv` | `Blue Box` | S1 E6 (unmarked), 1080p | matches |
| `[SubsPlease] 86 - Eighty Six - 02 (1080p) [285887DF].mkv` | `86 - Eighty Six` | E2 | matches `86 Eighty Six` (the trap from other tools is handled) |
| `[SubsPlease] One Piece - 1158 (1080p) [ABCD1234].mkv` | `One Piece` | S1 (unmarked), E1158 | matches the folder, but the season is guessed as 1 |
| `[SubsPlease] Boku no Hero Academia - 90 (1080p) [4B8B1261].mkv` | `Boku no Hero Academia` | S1 (unmarked), E90 | matches, season again guessed as 1 |

What this tells us:
1. The common cases already work. Trix and SubsPlease parse and match.
2. Judas fails safely, not wrongly. The parser keeps the `{English title}` and `[Uncensored]` in the title, so the folder match refuses and the file waits for a person. That is the "never guess" rule working, and it is also the cause of the Student Council holes if those episodes got stuck. Fix: strip `{...}` blocks and descriptive bracket tags like `[Uncensored]` from the title, and keep the `{...}` text as a synonym.
3. Absolute numbers are parsed but not mapped. The parser reports season 1 for `One Piece - 1158`. How the file gets placed afterwards depends on `placeOne` and the user's folder layout, which I have not tested. The catalog's absolute-to-season map is still needed.
4. Titles differ between groups: Trix puts `(Ao no Hako Season 2)` after the English title, and Judas puts the English title in braces after the romaji one. A catalog with synonyms fixes this properly: `Ao no Hako`, `Blue Box`, `Seitokai ni mo Ana wa Aru`, and `Even the Student Council Has Its Holes` all belong to one show.
5. Codec tags in the name are useful. Trix releases are AV1 already; Judas releases are HEVC 10-bit. The user's Tdarr flow converts to AV1, so Trix files should not need converting. Undermynd already reads codecs from the file header (the re-encode tab does), and Tdarr can hand back an original that needs no work. Confirm that an AV1 Trix file is skipped, not re-encoded.

## Numbering
- SubsPlease numbers some continuing shows absolutely (`One Piece - 1158`; `Boku no Hero Academia - 90` on a six-season series). Plex via TMDB files One Piece 1158 as season 23 episode 1158.
- Trix and Judas use `SxxExx`, which maps straight to a season.
- Other groups may batch or use other layouts; handle them as lower confidence.

## The holes problem (gap filling)
The user has shows where one group's releases skip episodes, so another group must fill them. Today that is manual. Native catalog support makes it a feature:
- A catalog episode list shows which episodes exist and which are missing, per season.
- Each show gets an ordered group preference, for example Judas first, then SubsPlease, then anything on nyaa that matches.
- When a gap appears, Undermynd searches for just that episode from the next group, not the whole batch.
- Do not download an episode already present from another group. Replacing one group's file with another's is a separate "upgrade" rule, off by default.
- Mixed groups in one season are fine for Emby, but their quality, codec and audio can differ. Show the group and resolution per episode in the library view.
- Check first whether the existing holes are actually releases that were skipped by the matcher (see item 2 above) rather than missing from the group's feed.

## nyaa.si and feeds
- The user already uses per-release feeds, which avoids the global-feed limit (reported as 75 items per feed by another downloader; not verified). Phase 0.5 should generate those per-show feeds from a monitored series so the user does not have to build each one by hand.
- Per-series queries (group + title + resolution) also make per-episode gap searches possible.
- SubsPlease has its own site with release lists and an airing schedule that third-party apps use; it is unofficial as an API, so treat it as an optional hint, not a dependency.

## Emby (and Jellyfin)
- Emby is already wired into Undermynd: playback filtering, Tracearr sessions, and the "need access?" account flow (Emby Connect). Jellyfin is supported in the access flow too.
- Nothing tells Emby to refresh after a file is placed (confirmed by the user). Emby finds new files on its own and that has been working.
- An explicit refresh is therefore optional: it makes a new episode appear right away. Build it as an off-by-default setting behind the media-server interface in the roadmap, Emby first. Check Emby's current API for the best call (targeted path update vs full refresh) before building; not verified here.
- Emby resolves metadata with its own providers, so keep the library naming compatible with it. The current placer writes `<Show>/Season N/<Show> - <ep> (<res>).ext`; check how Emby treats absolute episode numbers in that layout before changing anything.

## Implications for Phase 0.5
1. Build the test corpus from these real names (and more from the feeds) and fix Judas-style titles first: strip `{...}` and descriptive bracket tags, keep them as synonyms.
2. Match by title and synonym against a catalog, not by folder name.
3. Build absolute to season mapping and test placement for One Piece-style numbering.
4. Per-show ordered group preference with per-episode gap filling; 1080p as the global default.
5. Generate per-show nyaa feeds from monitored series; keep manual rules.
6. A "needs a person" list for anything unsure, with the reason shown (as the placer already leaves such files).
7. Use air dates so "missing" does not fire before a release exists.
8. Skip transcoding for sources already in the target codec (AV1).
9. Optional Emby/Jellyfin refresh after a placement.

## Open questions
- How are long-running shows laid out in the user's library (One Piece as season 1, season 23, or something else)? Decides how absolute numbers get placed.
- Are the Student Council holes episodes that never appeared in Judas's feed, or ones that appeared but were left unplaced because the title did not match?
- Does the Tdarr flow skip AV1 sources like Trix's, or re-encode them?

## Sources
- https://guyman624.github.io/thewiki/advanced/naming
- https://mc.tilde.club/~garret/old/seadex-muxing-standards.md
- https://community.firecore.com/t/tricky-episode-names-plex-understands-it/59643.md
- https://github.com/5rahim/seanime/issues/679
- https://www.github.com/Sher1234/Horrible
