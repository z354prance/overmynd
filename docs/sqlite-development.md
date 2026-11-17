# SQLite development repository

This is a Phase 1A foundation, not an application storage cutover. The executable
still opens its JSON store. `store.Repository` is context-aware and reports missing
jobs separately from storage failures; `LegacyRepository` keeps inherited callers
honest until their error handling is migrated. Never wrap SQLite read failures as
an empty legacy list or a missing legacy job.

## Dependency decision

Pin `modernc.org/sqlite v1.44.3`, a pure-Go SQLite 3.51.2 driver supporting Go 1.24.
Its driver documentation and go.mod were inspected in the downloaded module.
It preserves the existing CGO-disabled scratch-container build. No external SQLite
executable or server is required. `go mod verify` checks the pinned downloads.

The runtime dependency closure adds modernc libc/mathutil/memory, x/sys and x/exp,
plus go-humanize, uuid, go-isatty, go-strftime and bigfft. Keep libc at v1.67.6 to
match this driver's requirement; its documentation explicitly warns against version
mismatches. Tidy also records build/test module checksums (including ccgo, compiler
utilities and pprof); these are not new application services or runtime tools.
This is a compatibility/dependency review, not a claim of a security audit.

## Contract and schema

`OpenSQLite` takes an explicit local appdata path. The parent directory must already
exist and belong to the development instance. Newly created database files use
0600. Never pass a media-share or production database path.

Schema version 1 stores queryable job ID/state, a compatible JSON payload, nullable
catalog identity, an append-only transition journal and per-file import intents.
Newer unknown schema versions are rejected. Jobs are inserted once; updates run
under an immediate transaction, preserving protocol IDs and preventing stale
snapshot replacement. Callbacks only edit records and must not perform network or
filesystem operations. A returned updated job is published only after commit.

WAL, foreign keys, a 5-second busy timeout and FULL synchronous mode apply to every
connection. Each handle uses one connection; independent handles coordinate through
SQLite. Display history remains capped by Job.Move while the journal retains older
transitions. Import intents survive restart independently of job cleanup. The
repository does not copy, finalize, reconcile or verify files; recording `verified`
requires the eventual importer to establish that fact.

## Verified and remaining work

Docker tests cover failed SQL writes, actual SQLITE_FULL via max_page_count,
callback/commit cancellation, missing versus failed reads, independent competing
writers, history beyond the display limit and restart recovery of a copying intent.
These are temporary-file simulations, not a real media import.

`SQLite.ImportSnapshot` accepts already captured JSON bytes and migrates jobs and
their retained history in one transaction. It rejects null records, duplicate or
empty IDs, unknown states, inconsistent history and a nonempty destination. A
SHA-256 marker makes an identical retry a no-op, including after restart; a
different snapshot is rejected after the first import. The marker and its count
are committed with the jobs. Preserve the original capture separately. This API
does not capture a running JSON store, copy settings/secrets, submit downloads or
switch the executable to SQLite. Snapshot history can only include transitions
that the original JSON file retained.

Before application cutover: migrate all caller read errors; provide operator-facing
capture/import tooling and verify a complete backup/restore;
wire durable intents into an importer with reconciliation; exercise process crashes
around filesystem finalization. An intent table alone does not solve partial imports.
Do not copy a live SQLite database file as a backup, and do not read a changing
production JSON file. Full operational backup must also preserve secret.key and
settings; that is later migration/restore work.

Local measurement (Linux amd64, Go 1.24, CGO disabled, stripped build): executable
12,587,192 bytes. The network-isolated scratch-container smoke test used about
2.8 MiB idle memory and 0% CPU in a single Docker stats sample. The app was using
JSON during this measurement; it is not a SQLite workload benchmark. Measure again
when the application actually opens SQLite under representative load.

## Jobs-only backup API

`SQLite.Backup(ctx, destination)` uses SQLite's consistent `VACUUM INTO`
operation while the source remains open. It includes committed job payloads,
schema, transition journal, import intents and the one-time import marker.
Destinations are exclusively created with mode 0600; existing paths are refused.
Failure removes the newly owned incomplete output. The completed file is flushed.

Restore validation opens the backup as a separate development repository, checks
its integrity and compares payloads, journal, intents and import retry behavior.
This API is not yet operator-facing restore tooling or a complete operational
backup: settings, authentication files and secret.key still require a coordinated
backup procedure. It does not restore into a running application's database.
