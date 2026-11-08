# Docker installation and updates

Overmynd publishes `ghcr.io/z354prance/overmynd:latest` from `main` after tests pass.
The image supports Linux amd64 (including standard Unraid servers). `:main` tracks
the same release; `:auth-completion` is the development channel. Immutable
`sha-<full-commit-sha>` tags allow rollback to a particular published build.

## Docker Compose

Create a directory and save this as `compose.yaml`:

```yaml
services:
  overmynd:
    image: ghcr.io/z354prance/overmynd:latest
    container_name: overmynd
    restart: unless-stopped
    ports:
      - "18080:8080"
    environment:
      TZ: America/Chicago
    volumes:
      - overmynd_config:/config

volumes:
  overmynd_config:
```

Start it and open `http://YOUR-SERVER-IP:18080`:

```sh
docker compose up -d
```

Create your administrator account in Settings on a trusted network before exposing
the dashboard. Add integrations under Services. API keys remain on the server.
The dashboard is publicly readable; configuration and control require administrator
sign-in. Optional public media/access requests must be explicitly configured.

The container runs as UID/GID 1000. The named volume supplies a writable `/config`
directory. If you use a host directory instead, ensure UID/GID 1000 can write it.
Do not mount an existing configuration directory at another path.

## Docker command line

This uses a named volume and publishes the same port:

```sh
docker volume create overmynd_config
docker run -d --name overmynd --restart unless-stopped \
  -p 18080:8080 \
  -e TZ=America/Chicago \
  -v overmynd_config:/config \
  ghcr.io/z354prance/overmynd:latest
```

Use either Compose or the command-line installation, not both with the same
container name. Confirm health with `docker ps` and inspect logs with
`docker logs --tail 60 overmynd`.

## Updates and backups

Back up `/config` before updating. It contains the database, administrator account,
integration credentials and settings. Stop the container while copying this directory
so the database backup is consistent; protect the backup as sensitive configuration.
For a named volume, find its host location with `docker volume inspect overmynd_config`.
Compose prefixes volume names with its project name; `docker inspect overmynd` shows
the actual mounted volume. Restart the container after the backup.

For Compose, run from the directory containing `compose.yaml`:

```sh
docker compose pull
docker compose up -d
```

For a command-line installation, pull the image, stop and remove the old container,
then repeat the `docker run` command with the exact same mounts, network and environment.
Removing the container preserves the named volume. Do not remove the volume.

To roll back, use a previous published `sha-...` image tag. If database compatibility
requires it, restore the matching pre-update `/config` backup while the container is
stopped. Never run two instances against the same configuration directory.

## Existing Unraid installation

In Docker > Overmynd > Edit, set Repository to:

```text
ghcr.io/z354prance/overmynd:latest
```

Keep all existing settings, especially the mapping from
`/mnt/user/appdata/overmynd/config` to `/config`, custom IP/network, privacy filters
and optional media mounts. Click Apply. Future releases install through **Force
Update**. Switching the image tag does not copy or reset configuration.

The [Unraid template](../unraid/overmynd.xml) uses the stable image. See the
[Unraid guide](../unraid/README.md) for installation, icon and optional features.
On a custom IP, open `http://CONTAINER-IP:8080`; bridge port mapping `18080:8080`
does not change the port used on the container's own IP.

## Optional storage totals

Add a read-only media mount and enable the storage root:

```yaml
    environment:
      TZ: America/Chicago
      OVERMYND_STORAGE_ROOT: /media
    volumes:
      - overmynd_config:/config
      - /mnt/user/Media:/media:ro
```

Merge these entries into the existing service. Storage scanning reads category
sizes and filesystem capacity; no media write access or Docker socket is required.

## Reverse proxy

Use HTTPS for internet access. Preserve the external Host header and overwrite
X-Forwarded-Proto with the actual client scheme; restrict direct backend access.
Overmynd listens on container port 8080. A proxy must route to the reachable
container IP/port, or to the published host port when using bridge networking.
Initial administrator setup should already be complete before publishing the URL.

## Build from source

From a checkout of `main`:

```sh
docker build -t overmynd:local .
```

Use `overmynd:local` instead of the registry image in the installation command.
Local builds do not receive published updates through Unraid Force Update.
