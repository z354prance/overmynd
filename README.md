# Overmynd

A unified, read-only observability dashboard for self-hosted media automation stacks.

## Planned integrations

- Tracearr
- Radarr
- Sonarr
- Lidarr
- Seerr
- Tdarr
- qBittorrent
- NZBGet

## Media lifecycle

Seerr -> ARR -> qBittorrent/NZBGet -> Tdarr -> ARR Import -> Library -> Tracearr
# Authentication and deployment

Observability APIs are public and read-only. Services and Settings
provide administrator setup, login, logout, and authenticated configuration. Complete
initial setup on a trusted network before exposing a new instance. Concurrent setup
requests can create only one administrator.

All state-changing API requests, including setup, login, logout and connection tests,
require `X-Overmynd-Request: 1`. Browser clients send this automatically; command-line
clients must send it, plus their session cookie for administrator routes. Cross-origin browser writes are
rejected; CORS access is not enabled. JSON request bodies are limited to 64 KiB.

For HTTPS reverse proxies, preserve the external `Host` (including any port), overwrite
`X-Forwarded-Proto` with the actual client scheme, and restrict direct backend access.
This allows secure session cookies and origin checking behind the proxy. Direct HTTP
on a trusted Unraid LAN is supported. Use HTTPS outside that trusted network.

The `web/` assets mirror `internal/webui/web/`, which is embedded into the executable.
Update both together; CI checks parity, Go tests with the race detector, browser
authentication flows at 320–1440 px, and the Docker build.

To test on Unraid, pull `auth-completion` and build the Dockerfile. Keep the existing
`/config` mount (back it up first); the container runs as UID/GID 1000 and needs write
access. Verify public dashboard access while signed out, administrator setup or login
under Settings, service edits, logout, and HTTPS proxy access if used. PR #1 remains
a draft until deployment validation is complete.

## Public Seerr requests

The dashboard request box is an explicit, opt-in exception to admin-only control:
visitors can search and submit requests without an Overmynd account. Configuration
is still administrator-only, and requests are disabled until configured.

1. Add an enabled Seerr integration with its URL and API key under Services.
2. In Seerr, choose a dedicated user and set its request permissions, quotas, and
   auto-approval permissions. Leave auto-approval off if requests need review.
3. Sign into Overmynd, open Settings, and find Public media requests. Select the
   Seerr integration, enter the numeric user ID from that user's Seerr profile URL,
   enable public requests, and save. Saving verifies the selected Seerr identity.
4. Sign out and search from the dashboard. Movie requests require confirmation;
   TV requests explicitly confirm all seasons. Seerr decides approval using the
   configured user's current permissions. Requests stay off the active-media cards
   until downloading or processing starts.

Overmynd keeps the API key on the server and sends Seerr's `X-API-User` header on
every search/request. It never accepts a requester ID, approval override, or quota
bypass from public callers. A missing or unavailable configured user fails closed.
Submissions are limited to 10 per minute per direct client IP; visitors behind the
same reverse proxy share this additional limit. Seerr's own quotas also apply.

API reference: [Seerr request endpoint](https://docs.seerr.dev/api/create-new-request/)
and [Seerr user middleware](https://github.com/seerr-team/seerr/blob/develop/server/middleware/auth.ts).

Navigation opens with the menu button and closes on selection, Escape, or a backdrop
click. Now Playing and Playback load Tracearr's relative image-proxy URLs through a
restricted Overmynd route; missing artwork displays a play placeholder. The image
route accepts only signed Tracearr image paths and never forwards API credentials.
