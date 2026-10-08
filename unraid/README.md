# Unraid template

This template pre-fills the existing configuration path and dashboard port. It does not download, replace, or publish your private application settings. It uses the locally built `overmynd:auth-completion` image; no registry image is published by this template.

## Install from the existing checkout

Run in the Unraid terminal:

```bash
cd /mnt/user/appdata/overmynd &&
git pull --ff-only origin auth-completion &&
docker build -t overmynd:auth-completion . &&
mkdir -p /boot/config/plugins/dockerMan/templates-user &&
cp -n unraid/overmynd.xml /boot/config/plugins/dockerMan/templates-user/my-overmynd.xml
```

The copy does not overwrite an existing user template, which may contain customized ports or paths. If you already created Overmynd through Unraid, keep managing that existing container through Edit.

For a first migration from the command-line container, after the build and template installation succeed:

```bash
docker stop overmynd-dev &&
docker rm overmynd-dev
```

This removes only the old container, leaving the image and configuration directory in place. In **Docker → Add Container**, select **overmynd** from the user templates. Confirm these values, click **Apply**, and enable **Autostart** on the Docker page:

- Repository: `overmynd:auth-completion`
- Host port `18080` → container port `8080` (TCP)
- Host path `/mnt/user/appdata/overmynd/config` → container path `/config` (read/write)

The WebUI remains at `http://SERVER-IP:18080`. Do not run the old and new containers simultaneously against the same configuration directory.

## Updates

Pull and rebuild the image in the same checkout. Then recreate the container through Unraid using its saved template. The template uses a local image, so registry-based Update/Force Update is not the image build mechanism. Do not delete the image or the appdata directory. Keep the configuration mapping unchanged to preserve the database and administrator account.

Unraid stores user templates under `/boot/config/plugins/dockerMan/templates-user`; see the [Unraid documentation](https://docs.unraid.net/community-applications/). Adding this file to the repository does not automatically list the app in Community Applications.

## Icon for an existing container

The template includes the project icon. If you installed an earlier template, open the existing container's **Edit → Advanced View**, set **Icon URL** to the following, and click **Apply**:

```text
https://raw.githubusercontent.com/z354prance/overmynd/auth-completion/unraid/overmynd-icon-v2.png
```

Refresh the Docker page afterward. You do not need to rebuild the application image or replace your saved configuration to add the icon.

## Storage footer

Optional public read-only totals are enabled with `OVERMYND_STORAGE_ROOT=/media` and a read-only path mapping `/mnt/user/Media` to `/media`. Existing Unraid containers need both entries added through Edit; pulling the repository does not modify saved templates. Leave the variable blank to disable totals.

The five folders are `Movies`, `Episodes` (TV), `Anime`, `Music`, and `Books`. Other folders are excluded. Categories show logical file sizes (hard-linked files are counted per directory entry), not allocated disk blocks. Free space comes from the media mount's filesystem, so on Unraid shfs it reflects the capacity reported by that share filesystem, potentially including pools. It is not calculated by subtracting these five categories. Compare `df -h /mnt/user/Media` with the intended array capacity.

A background scan starts on the first storage request and refreshes no more than every 30 minutes while the dashboard is used. Large libraries may take time and can wake disks. The dashboard never waits for a scan; it shows the last result while refreshing. Missing/unreadable folders show unavailable, empty folders show zero. Symlinks are skipped. The API exposes totals only, never paths, filenames, or error details. Configuration is through the administrator-managed container environment, not a public write endpoint.

The footer shows a small used-space ASCII meter with percentage remaining. Click it for category sizes, total capacity, and remaining space. The pop-out closes with Escape, its Close button, or a click outside.

## Embedding in the xivix portal

Overmynd permits framing by `https://xivix.cc`, `https://www.xivix.cc`, and its own origin using CSP `frame-ancestors`. Other parents are rejected. This does not grant the parent access to administrator APIs or relax CSRF checks. The reverse proxy must not add `X-Frame-Options: DENY/SAMEORIGIN` or a stricter `frame-ancestors` policy; multiple CSP policies are intersected by browsers. The portal must allow `https://overmynd.xivix.cc` in its own frame-src policy. If it uses a sandbox attribute, scripts, forms and same-origin behavior must be allowed for interactive requests/sign-in.


### Hide a private Emby library from Now Playing

For a single-Emby-server deployment, add the container variable
`OVERMYND_HIDDEN_EMBY_LIBRARIES` with comma-separated library IDs (for example `9`).
Leave it empty to disable filtering. Existing Unraid containers need this variable
added through Edit; updating the repository template does not update saved containers.

Tracearr sometimes omits `library_id`. Overmynd then uses its saved access-request
Emby URL and API key to resolve the session's `rating_key` through Emby's read-only
`/Items/{Id}/Ancestors` endpoint and match a library root. These credentials must
point to the same Emby server as Tracearr. The access-request form need not be enabled.
This filter is not intended for multiple Emby servers with overlapping item/library IDs.
Unidentified sessions, missing server identities, failed lookups, and unconfigured
Emby credentials are withheld when their privacy cannot be verified. Explicit
non-Emby sessions remain visible. Lookups have an eight-second total budget per refresh.

Filtering happens before the public playback response and poster URLs are generated;
hidden sessions do not contribute to the Now Playing count. No Emby permissions,
playback, or library contents are changed. This controls Now Playing only, not
Recently Added or download/processing sections. Previously fetched browser data
cannot be recalled; reload open dashboards after enabling the filter.


### Series overview tracking

A series that enters the download/import pipeline, or is identified from an active
or failed Tdarr episode job, is remembered in the configuration database. Its overview
stays after queue entries disappear and across restarts, until Sonarr reports files
for every monitored episode. Pending-only Tdarr jobs and the missing-series library
are not used to enroll shows. Counts always come from Sonarr, not download percentages.
Missing or unaired monitored episodes keep an enrolled series unfinished; unmonitored
episodes do not count. Tdarr recovery matches an exact normalized show title (including
year when present) before episode coordinates, avoiding ambiguous title matches.
