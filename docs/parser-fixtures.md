# Phase 0 release-name corpus

`internal/library/testdata/roadmap_releases.json` preserves supplied roadmap names
and explicitly labelled synthetic edge cases. The parser test captures current
behavior, including limitations; it does not certify these files for automatic
placement or claim the releases are currently available in any feed.

Coverage includes Trix seasonal numbering, Judas aliases/descriptive tags,
SubsPlease names, numeric titles, absolute numbering, numeric hashes, a season
pack, an episode range and a dimensions-only quality tag. Every entry records
its provenance. Synthetic names are not presented as observed user downloads.

A fixture season of zero means unresolved numbering. The test deliberately does
not approve the current parser's implicit season 1 when `Marked` is false. Title
matching alone is not a safe season mapping. Judas currently remains unmatched;
its limitation is recorded rather than weakening matching to make the test pass.

This is a characterization baseline, not the Phase 2 parser contract. When aliases,
pack coverage and explicit numbering mappings are implemented, evolve these tests
alongside the domain model. Actual import tests must still prove no overwrites,
partial placement recovery, duplicate acquisition protection and real Emby layout
recognition. Existing synthetic pipeline tests are not substitutes for these gates.
