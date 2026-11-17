<p align="center"><img src="assets/undermynd-logo.svg" width="160" alt="Overmynd"></p>

# Overmynd

Rework baseline: copied from Undermynd for the upcoming Overmynd overhaul.
Automatic Docker publication is paused; this is not yet a switch-over release.

Start with the [active rework roadmap](docs/roadmap/README.md) and
[implementation handoff](docs/roadmap/handoff.md). The feature descriptions below
describe the inherited baseline, not completed rework milestones.

One container that owns the whole download-to-library path for Sonarr, Radarr and Lidarr.

- Acts as their **qBittorrent** download client, so a grab keeps the same download ID from request to import.
- Hands the grab to a local qBittorrent (TorBox, NZBGet, a fake SABnzbd and a Tdarr hand-off are also built; RSS is next), keeps only the media files, moves them atomically into the complete folder, confirms the arr's import, then cleans up everything it created.
- A public status page for your users, and a sign-in-protected admin side, in one app on one address.
- Standard library only. One static binary of about 7 MB, about 7 MB of memory at idle.

## Status

| Phase | Scope | State |
|---|---|---|
| 1 Core | Job table and state machine, fake qBittorrent API, local qBittorrent backend, staging, hand-back, import confirmation, cleanup, status page, admin sign-in | **built and tested** |
| 2 TorBox | Usenet and torrents (limiter, batched polling, resumable pull, tracked delete) | built and tested against a stand-in; not yet run against real TorBox |
| 3 Transcode | Folder hand-off to a dedicated Tdarr library, verification gate | folder hand-off built and tested against a simulated Tdarr; not yet run against real Tdarr |
| 4 Usenet | Fake SABnzbd, TorBox usenet backend, NZBGet fallback | built and tested against stand-ins; not yet run against real TorBox, NZBGet or an arr |
| 5 RSS | Feeds, regex rules, first-poll baseline, dedupe, magnet/torrent/NZB items | built and tested against a stand-in feed |
| 6 Hardening | Notifications, API tokens, more admin actions | partly (reconciler, retries and cleanup are in) |
| 7 Overmynd merge | Requests, access requests, playback | not started |

The design is in `docs/`.

## Run

Use the Unraid template in `unraid/overmynd.xml`, or:

```
docker run -d --name overmynd -p 8080:8080 -p 8081:8081 \
  -v /mnt/user/appdata/undermynd/config:/config -v /mnt/user/data:/data \
  -e PUID=99 -e PGID=100 ghcr.io/z354prance/overmynd:latest
```

1. Read the container log. On first start it prints the **download client login** for the arrs and a **setup key**.
2. Open `http://host:8080/admin`, enter the setup key, choose a password, scan the authenticator secret, confirm a code, and save the recovery codes.
3. Under *settings*: set each category's arr URL and API key (the arr that confirms the import), and the local qBittorrent URL and login.
4. In Sonarr, Radarr and Lidarr add a **qBittorrent** download client: host = this machine, port = **8081**, the login from step 1, and the category (`radarr`, `tv-sonarr`, `lidarr`).

Mount the same `/data` in Overmynd, qBittorrent, the arrs and Tdarr, on one cache-only share, so every move is a rename.

## Ports

| Port | Purpose | Exposure |
|---|---|---|
| 8080 | Public status page, and the admin pages under `/admin` behind a password and TOTP | Reverse proxy, open to users |
| 8081 | Download client API for the arrs | LAN only. Never expose it. |

Behind a reverse proxy set `UNDERMYND_TRUST_PROXY=true` so the real client IP is used for sign-in throttling and the origin check.

## Folders

```
/data/undermynd/client/<job>/     local qBittorrent saves here (removed after the media is staged)
/data/undermynd/staging/<job>/    media files only
/data/complete/<category>/<release>/   what the arr imports from; removed after the import is confirmed
```

## Security notes

- The public side reads only a public view (title, state group, progress). No paths, IDs, reasons or settings.
- Every admin route and API call passes one session check before any handler runs. State-changing calls also need a custom header and a same-origin check.
- Sign-in needs the password **and** a TOTP code (or a one-time recovery code), with backoff on repeated failures. The error never says which part was wrong.
- Stored secrets (arr API keys, qBittorrent password, the client login) are encrypted with AES-256-GCM using a key in `/config/secret.key`, and no API returns them.
- First-run setup needs the setup key from the log, so nobody who merely reaches the page can claim the account.
- Passwords use PBKDF2-HMAC-SHA256 (600,000 rounds), from the Go standard library.

## Known gaps

- The arr history check (`downloadId` + import event) is written from the arr API as documented and tested against a stand-in; confirm it against your real Sonarr, Radarr and Lidarr. Set `UNDERMYND_LOG_REQUESTS=1` to log every call the arrs make to the fake client and compare.
- The job file is a small JSON file written atomically after every change, not SQLite, because this build uses only the standard library.
- The release folder in `complete/` is named after the release (not the job ID) so the arrs can parse it.

## Branding

Logo files are in `assets/`; the Unraid icon is `unraid/undermynd-icon.png`.

## Usenet (SABnzbd side)

Add Overmynd to Sonarr/Radarr/Lidarr as a **SABnzbd** client on the machine port (host 18081 by default).
The **API key is the client password** (shown once in the log on first run, rotatable in the admin page).
Usenet goes to TorBox when a token is set, otherwise to NZBGet when its address is set.

## Tdarr hand-off

For categories with transcode on, finished videos go to the Tdarr **input folder** and Overmynd collects the result from the **output folder**. Create a dedicated Tdarr library that reads the input folder and writes folder-to-folder into the output folder. Overmynd never touches any other library.

With a Tdarr address (and API key, if Tdarr has authentication on) and the library's ID set on the Transcode tab, Overmynd also:
- asks Tdarr to scan just that job's folder, so timing does not depend on a folder watcher;
- shows the node, worker and percentage while a worker holds the file (`get-nodes`, and `cruddb` file records, the same calls Overmynd uses);
- fails the job at once when Tdarr marks the file errored, and treats a worker with no progress for the stall time as failed;
- keeps at most N jobs with Tdarr at once (default 6); the rest wait, which is not a failure;
- hands back the original when Tdarr decides a file needs no work.

Per category, a failed transcode either fails the grab (default, so the arr blocklists and searches again) or keeps the original.
The output is accepted only when it has stopped growing, passes a size check and has a valid container header. The design asked for an ffprobe duration check too, which needs ffprobe in the image and is not built yet.
Without a Tdarr address everything still works from the folders alone, but failures are only noticed when the timeout runs out.

## Folder layout

By default everything lives under the one `/data` mount: work files in `/data/undermynd`, hand-back folders in `/data/complete/<category>`.
If your layout differs, set `UNDERMYND_WORK` (staging, Tdarr hand-off, local client) and `UNDERMYND_COMPLETE` (what the arrs import from).
Keep both inside one mount of one cache-only share so every move is a rename.


## Log

Admin → Log shows the last 2000 lines of the container log since Overmynd started, with a filter box (a job id, `tdarr`, `failed`) and a follow switch. The one-time setup key is never kept in it.

## Resolution check

When a download finishes, Overmynd reads the real frame size of each video from its container header and asks the arr (Sonarr or Radarr, through the download's history) what quality it grabbed the release as. If the video is below that step (2160, 1080, 720, 480), such as a release labelled 1080p that is really 480p, the job fails and the arr is told to blocklist the release and search again. Wider-than-tall crops count by width, so a 1920x800 film is 1080p. If the arr cannot say, cannot be reached, or the frame size cannot be read, the download goes on as normal. Turn it off under Admin → System → Timing ("check resolution against the arr").

## Re-encoding files already in the library

The admin **Re-encode** tab lists videos in `Movies`, `Episodes` and `Anime` under the media share (`UNDERMYND_STORAGE_ROOT`, mounted at `/media`) that are not AV1 and arrived since a date you choose. The codec is read from the file's own header, so nothing needs ffprobe. Picked files are copied to staging, sent through the same Tdarr flow as a new grab, and the result is checked (valid container, read back as AV1) before it is written beside the original and moved over it. If Tdarr fails, produces anything that is not AV1, or the library file changes meanwhile, the library file is left as it was. These jobs never touch Sonarr or Radarr and are not shown on the public page; run a rescan in Sonarr or Radarr afterwards so it picks up the new files.

This is the only thing that writes to the media share, so the share has to be mounted read-write.

## Continuing from Undermynd

This repository now continues the Undermynd application as Overmynd. Existing
Undermynd configuration files, secret keys, jobs, client credentials and working
paths remain compatible. Keep your current /config, /data, /media and other mounts,
ports and environment settings when switching to the Overmynd image.
Do not substitute an old dashboard-only Overmynd configuration for the current
Undermynd configuration. Back up the current configuration before changing images.

Existing UNDERMYND_* variables remain supported. OVERMYND_* equivalents take
precedence when supplied. The default work folder and download-client username
retain their existing values so in-flight downloads and arr connections continue
using the same paths and credentials. Configure Sonarr/Radarr/Lidarr against the
existing machine port (8081); the public/admin port remains 8080.
