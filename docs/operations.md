# Operations

## Configuration

`config.toml` in the directory passed with `--config` (created with defaults on first start). Changes to `logLevel`, `logPath` and `checkForUpdates` are picked up live; everything else needs a restart.

| Key | Default | Meaning |
|---|---|---|
| `host` | `localhost` | listen address; use `0.0.0.0` when clients connect directly without a reverse proxy |
| `port` | `8282` | listen port |
| `baseUrl` | `/` | path prefix when served under a sub-directory of a reverse proxy |
| `sessionSecret` | generated | secret for the web UI session cookie |
| `secureCookie` | `false` | mark the session cookie `Secure`; enable only behind HTTPS |
| `logLevel` | `DEBUG` | `ERROR`, `WARN`, `INFO`, `DEBUG`, `TRACE` |
| `logPath` | empty | log file path; empty logs to stdout only |
| `logMaxSize`, `logMaxBackups` | `50`, `3` | log rotation (MB, files) |
| `checkForUpdates` | `true` | periodic GitHub release check |
| `databaseType` | `sqlite` | `sqlite` or `postgres` |
| `postgresHost`, `postgresPort`, `postgresDatabase`, `postgresUser`, `postgresPass`, `postgresSslMode` | see template | PostgreSQL connection |
| `syncMaxBodySizeMB` | `64` | largest sync upload accepted (`413` above it); `0` disables the limit |
| `syncHistoryLimit` | `10` | previous sync payloads kept per API key for rollback; `0` disables history |

Sizing note: with history enabled the database holds up to `syncHistoryLimit + 1` rendered payloads per API key plus one row per manga/chapter/category in `sync_item`. A large library is a few MB per payload and a similar amount of item rows.

## Databases

- **SQLite** (default): `syncyomi.db` next to `config.toml`, WAL mode, no extra setup. Fine for personal use.
- **PostgreSQL**: set `databaseType = "postgres"` and the `postgres*` keys. `docker-compose.yml` in the repository shows a working pairing. The database must be owned by the configured user (PostgreSQL 15+ no longer grants `CREATE` on `public` to everyone).

Schema migrations run automatically at start-up, inside one transaction. SQLite tracks the version in `PRAGMA user_version`, PostgreSQL in `schema_migrations`. Downgrading the binary below the schema version is refused.

## Backups

Back up the config directory (SQLite) or take a PostgreSQL dump. Sync payloads are stored as opaque blobs; there is nothing else to preserve.

## Rolling back sync data

Every upload keeps the previous payload (up to `syncHistoryLimit`). In the web UI, open *Settings → API Keys → Details* for the key and pick *Restore* on an older entry. The restored payload becomes current under a new ETag; each device downloads it on its next sync (a device that tries to upload with a stale `If-Match` gets `412` and re-syncs). Each entry can also be downloaded; entries marked *device upload* are complete backup files the apps can import directly.

The same view shows which devices have synced with the key, when they were last seen and the last status they reported. Devices appear once they report sync events or send the `X-Device-ID`/`X-Device-Name` headers.

## Upgrade notes

### 1.6.0

- The v1 endpoints store and serve client uploads byte-for-byte again, as before 1.3.0:
  `PUT /api/sync/content` keeps the exact bytes and answers at once; the import into the
  item store follows in the background, or when a v2 device next needs it. `GET` echoes
  the bytes until a v2 device writes. v1 ETags return to
  `uuid=…`. Payloads that do not decode are accepted and served unchanged, like 1.1.x.
- A pre-1.3 payload that cannot be decoded is no longer answered with 404 or overwritten by
  a later upload; it is served verbatim until a device pushes a fresh library.
- Equal-version conflicts now fall back to the payloads' `last_modified_at`, so clients that
  never bump `version` (v1-era builds) propagate changes instead of being stuck at their
  first upload forever.
- If a library got stuck while syncing v1 clients through 1.3/1.4: restore a history entry
  with a `uuid=…` etag (a device upload) from *Settings → API Keys → Details*, or push a
  full library from the device with the most complete state.
- Large libraries: a v1 upload is answered in well under a second whatever its size. In
  1.3–1.5 the import ran inside the request and a library of a few thousand manga exceeded
  the 10 s timeout of Komikku's sync client on every sync (#225). Each import now logs
  `imported v1 upload into the item store` with its duration.
- The HTTP server has timeouts: 15 s to send request headers, 10 min per request, 2 min
  for an idle keep-alive connection. The event stream is exempt.
- Schema: `sync_data` gains `raw_data`/`raw_etag`/`raw_seq`/`raw_pending`, `sync_item`
  gains `modified_at`, `sync_status` gains `last_protocol`. The migration runs
  automatically.
- Web UI: keys with v1 devices are flagged with a banner and *legacy* chips; devices show
  how many merges they are behind; history entries show their origin and can be downloaded;
  stale devices can be forgotten. New admin endpoints `DELETE …/devices/{id}` and
  `GET …/history/{id}/download`.

### 1.3.1

- Sync timestamps are now stored in UTC. On Postgres, servers running with a non-UTC `TZ` used to store the local wall clock in the timezone-less `TIMESTAMP` columns, so the web UI showed times shifted by the host offset. Device and status rows correct themselves on the next sync event; older history entries keep the skewed time until they roll off `syncHistoryLimit`.

### 1.3.0

- New tables `sync_state` and `sync_item`; `sync_device` gains `last_cursor` and `protocol`, `sync_data` gains `rendered_seq`. The migration runs automatically.
- Existing sync payloads are imported into the item store the first time their API key syncs; the log shows `imported legacy sync payload`. A payload that cannot be decoded keeps being served to v1 clients unchanged and is logged as an error.
- ETags change from `uuid=…` to `seq=…`; clients simply re-download once.
- Protocol v1 is deprecated (`Deprecation` header, a daily warning per device in the log, *legacy* badge in the web UI) but fully supported. See [clients.md](clients.md) for the compatibility matrix.
- *Restore* in the web UI now rebuilds the item store from the chosen version; all devices receive the restored library on their next sync.
- Large libraries: an upload of a full library with tens of thousands of chapters takes well under a second on SQLite; the first sync after the upgrade may take a little longer while the payload is imported.

### 1.2.0

- New tables `sync_data_history`, `sync_device`, `sync_status` (migration runs automatically).
- New config keys `syncMaxBodySizeMB` (default 64) and `syncHistoryLimit` (default 10). Set `syncMaxBodySizeMB = 0` if you have unusually large libraries and see `413` responses.
- Deleting an API key now also removes its sync data, history, devices and status (previously the payload row was left behind on SQLite).
- `PUT /api/sync/content` accepts `Content-Encoding: gzip`; `GET` answers with gzip when the client accepts it.
- The `/api/sync/admin/*` endpoints are only reachable with a web UI session, not with an API key.
- `OpenAPI.yaml` now describes the endpoints that exist; the old `/device` and `/sync/{apiKey}` entries were never implemented and have been removed.
