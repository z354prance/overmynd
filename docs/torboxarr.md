# TorBoxarr transfers

Add a service while signed in as an Overmynd admin:

- Type: **TorBoxarr - Usenet (SABnzbd)**
- Name: **TorBoxarr**
- URL: `http://10.0.0.77:8085` (or your own TorBoxarr address)
- API key: the key accepted by TorBoxarr's SABnzbd API

Save and test the connection. The credential stays in Overmynd's server-side service credential storage; public responses never include it. Read-only queue calls send the key in a form body, not the URL. Redirects are refused and errors are sanitized.

The dashboard shows **TorBox to Unraid** transfer progress before Tdarr processing. TorBoxarr's SABnzbd queue reports local bytes rather than the upstream TorBox download percentage. Queued cloud work stays hidden under the dashboard's existing pending-item filter. Moving/verifying remains part of the transfer stage until TorBoxarr removes it from the queue. Completed records are omitted.

Progress uses `mb` and `mbleft`, with `percentage` as a fallback when available. Missing metrics stay unknown. Per-item `kbpersec`, if supplied, is displayed as speed; a queue-wide speed is never attributed to individual jobs. Current TorBoxarr versions may provide ETA but no speed. For torrent jobs, add a second service as described below.

Implementation reference: https://github.com/MrJoiny/TorBoxarr/blob/main/internal/compat/sab.go

## Both Usenet and torrents

Add two services using the same base URL, such as `http://10.0.0.77:8085`:

| Type | Credentials |
| --- | --- |
| TorBoxarr - Usenet (SABnzbd) | `TORBOXARR_SAB_API_KEY` |
| TorBoxarr - Torrents (qBittorrent) | TorBoxarr qBittorrent username (usually `admin`) and `TORBOXARR_QBIT_PASSWORD` |

Existing TorBoxarr services remain Usenet services with their saved keys. Add the torrent service separately; do not replace the key with a password. Both connections store credentials server-side, and their jobs appear in the same dashboard. Torrent speed comes from the per-job local transfer speed; cloud-queued jobs stay hidden. Completed torrent jobs are omitted, and verification stays in the transfer stage.
