# M0 runtime contract and operational decisions

This is the historical M0 checkpoint contract. Current M1 behavior is documented
in [m1-contract.md](m1-contract.md) and [m1-development.md](m1-development.md).

M0 is local-only development infrastructure using synthetic/disposable data. It
implements no domain API, authentication, authorization, audit, or search. These
choices implement ADRs 0001–0003; they do not change the architecture baseline.

## Endpoint contract

These infrastructure endpoints are outside `/api/v1` and its future authenticated
domain contract. They require no authentication in M0 and bind only to loopback.

| Request | Response |
| --- | --- |
| `GET /healthz` | `200`, `{"status":"ok"}` while the HTTP process is serving |
| `GET /readyz` | `200`, `{"status":"ready"}` after initialization and a successful bounded SQLite migration-metadata read |
| `GET /readyz` while unavailable/stopping | `503`, `{"status":"not_ready"}` |
| `HEAD` on either endpoint | Same status/headers as GET, no response body |
| Other methods on either endpoint | `405`, `{"status":"method_not_allowed"}`, `Allow: GET, HEAD` |
| Other paths (including `/api/v1/*` and `/mcp`) | `404`, `{"status":"not_found"}` |

JSON bodies end with a newline. Responses use `application/json`, `Cache-Control:
no-store`, and `X-Content-Type-Options: nosniff`. No configuration, paths, database
versions, SQL errors, credentials, request contents, or system diagnostics appear
in responses. Probes do not write application data and require no idempotency key.
Readiness checks have a 250 ms context deadline; SQLite lock waits have the separate
bounded busy timeout below. A listener starts only after successful migrations.

## Configuration and logs

Only process environment is read; `.env` is an example, not automatically loaded.
Unset values use the defaults below. Explicit empty/invalid values fail startup.

| Variable | Default | Validation |
| --- | --- | --- |
| `CONTEXTARIUM_LISTEN_ADDR` | `127.0.0.1:8080` | Numeric loopback IP (IPv4 or IPv6) and port 0–65535; `localhost` maps to `127.0.0.1`. Port 0 requests an OS-selected local port. Wildcard/non-loopback addresses are rejected in M0. |
| `CONTEXTARIUM_DB_PATH` | `./data/contextarium.db` | Filesystem path, relative to the working directory or absolute; not a SQLite URI or in-memory database. |
| `CONTEXTARIUM_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` (case-insensitive) |

Use the standard-library `log/slog` JSON handler on stderr. Log lifecycle events
and safe failure stages, not raw request headers/bodies, environment/configuration
dumps, database contents, or raw filesystem/driver errors. No request/access logger
is installed. HTTP library diagnostics are discarded rather than echoing hostile
request data. M0 requires no secrets.

## SQLite baseline

- Driver: pinned [`github.com/mattn/go-sqlite3`](https://github.com/mattn/go-sqlite3), through `database/sql`. This is the
  sole external Go module; its bundled SQLite avoids a runtime SQLite installation.
  Builds require Go 1.26+, `CGO_ENABLED=1`, and a C compiler (GCC/Clang). Build on
  Linux for Linux, or supply an appropriate C cross-compiler. Standard builds use
  the bundled amalgamation, not the `libsqlite3` tag. The binary still relies on
  the target platform's normal C runtime; it is not claimed to be fully static.
- M0 needs ordinary SQLite SQL, foreign keys, and transactions. No FTS tables or
  search behavior are enabled. The driver's optional FTS5 build support can be
  selected and tested in M5.
- One open connection and one idle connection; no connection expiry. DSN options
  initialize every replacement connection: foreign keys ON, WAL journal, synchronous
  FULL, busy timeout 1000 ms, private cache, and IMMEDIATE transactions.
- Initialization verifies those PRAGMA values. WAL + FULL favors durability over
  peak write throughput. SQLite serializes writers; this is not a multi-process
  deployment design. Use a local filesystem and an owner-controlled data directory.
- Create missing parent directories with mode `0700` and a new database with mode
  `0600` (subject to umask). Existing files must be regular files, not symlinks.
  Existing directory/file permissions are not silently changed. Operators remain
  responsible for existing paths and local filesystem access. SQLite manages WAL
  and shared-memory sidecars; do not copy only a live main database as a backup.
- Startup has a 10-second context deadline. Lock waiting is bounded by the busy
  timeout and context cancellation; startup fails rather than retrying forever.
- On shutdown, withdraw readiness, drain HTTP for up to five seconds, force-close
  remaining connections if needed, then close the SQLite pool. Pool closure waits
  for database operations already in progress to finish.
  Startup failures also close acquired listeners/database resources.

## Migrations

Migrations are ordered SQL embedded in the binary, not supplied by clients or
loaded from the working directory. Version 1 creates only `schema_migrations`.
The ledger records version, name, SHA-256 of the embedded SQL, and application time.
The checksum detects accidental migration editing; it is not tamper-proof audit.

All pending SQL and ledger updates run in one IMMEDIATE transaction. Repeat startup
validates the existing ledger and performs no duplicate migration. A failed batch
rolls back its SQL and bookkeeping; startup fails before opening the HTTP listener.
A failed first startup may leave an empty database/directory for a corrected retry.

Unknown/newer versions, gaps, changed migration names/checksums, malformed metadata,
and nonempty databases without the ledger fail closed. There is no automatic
downgrade, destructive reset, or adoption of an unrelated database. Connection
PRAGMAs may already have initialized journal mode before an incompatibility is
reported; no migration/schema downgrade is performed.

## Developer commands

```sh
go mod download
make fmt
make vet
make test
make build
./bin/contextarium
```

In another terminal:

```sh
curl --fail http://127.0.0.1:8080/healthz
curl --fail http://127.0.0.1:8080/readyz
```

Stop the binary with Ctrl-C or SIGTERM. `make race` runs the race-enabled tests.
Tests use temporary synthetic databases and loopback listeners; no cloud service
is required. Module download requires access to the Go module proxy/checksum
service on first setup. Runtime and tests do not call external services.

No M1+ feature is implemented. Linux deployment, full crash/power-loss durability
rehearsal (M2), search capability checks (M5), and backups/restores (M9) remain
separate validation steps; M0 tests do not establish production readiness.
