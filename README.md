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

The dashboard and observability APIs are public and read-only. Services and Settings
provide administrator setup, login, logout, and authenticated configuration. Complete
initial setup on a trusted network before exposing a new instance. Concurrent setup
requests can create only one administrator.

All state-changing API requests, including setup, login, logout and connection tests,
require `X-Overmynd-Request: 1`. Browser clients send this automatically; command-line
clients must send it with their session cookie. Cross-origin browser writes are
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
