# Development jobs maintenance

These commands operate only on explicitly selected Overmynd development files.
Do not use Undermynd's files, shared appdata, a running JSON store or production
download accounts. The main application still uses JSON; this tool does not
switch it to SQLite or resume any imported acquisition.

Build from the work branch with Go 1.24:

```sh
go build -o overmynd-db ./cmd/overmynd-db
```

Place a separately captured immutable JSON job snapshot and a preserved backup
in your isolated development directory. Keep settings and secret.key together
separately. Do not point this tool at a changing source to create the capture.

Import into a development database whose parent directory already exists:

```sh
./overmynd-db import -database /dev-appdata/jobs.db -snapshot /dev-captures/jobs.json
```

Output is JSON with `imported` and the committed `jobs` count. Repeating the
identical snapshot returns `imported:false`; a different snapshot or a populated
destination without that marker is rejected. Job IDs and remote IDs are preserved.
Errors exit nonzero. The command does not load application configuration, read
credentials, launch the pipeline or submit backend requests.

Create a consistent backup while including committed WAL data:

```sh
./overmynd-db backup -database /dev-appdata/jobs.db -output /dev-backups/jobs.db
```

The source must exist as a regular SQLite file; a typo cannot silently create an
empty database. The output must not exist. `backedUp:true` is reported only after
backup and flush succeed. Keep the completed backup immutable. For isolated
restore testing, copy that completed backup to a new development path before
opening it; never copy a live source database or replace a running database.

Automated command tests import/retry, back up, reopen independently and verify
the protocol IDs and migration marker. Repository-level restore tests additionally
check integrity, full journal and per-file intents. These are jobs-only fixture
tests. Full operational capture/restore of settings, secret.key, authentication
and other files, plus application read-error propagation and importer recovery,
remain phase gates.
