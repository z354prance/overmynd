# Undermynd replacement baseline

This branch replaces Overmynd application sources with the contents of
`z354prance/Undermynd` at commit `0f46f7b` (main), preserving Overmynd Git history.
Application names, module paths and Undermynd deployment defaults are intentionally
retained for the next roadmap. The publishing workflow targets the Overmynd image
package so this repository cannot overwrite Undermynd releases.

This is a replacement baseline, not a compatible update of the old dashboard.
Do not point it at the existing Overmynd configuration or replace a running
container without a migration plan. It uses a different configuration format,
needs writable download paths and adds a LAN-only machine API on port 8081.

The stable main branch/image remains unchanged while this branch is reviewed.
The imported Linux filesystem ownership code needs Linux for its complete tests;
GitHub CI runs the suite and Docker build on Linux. Deployment and configuration
migration details will be finalized with the requested roadmap.
