# HTTP API

Base path: `/api`. The machine-readable version of this document is [`OpenAPI.yaml`](../OpenAPI.yaml).

## Authentication

| Method | Used by | How |
|---|---|---|
| API key | sync clients | `X-API-Token: <key>` header (also accepted as `?apikey=<key>`) |
| Session | web UI | `user_session` cookie set by `POST /auth/login` |

Most endpoints accept either. `/sync/admin/*` accepts the session only: an API key must not be able to inspect another key's data.

## Sync v2 (clients)

See [sync-protocol-v2.md](sync-protocol-v2.md) for the protocol.

| Endpoint | Purpose |
|---|---|
| `GET /sync/v2/capabilities` | `200 {"version":2,"merge":…,"snapshot":…}`; old servers answer `404` |
| `POST /sync/v2/merge` | merge the uploaded backup, receive what the client lacks (`X-Device-ID` required; `X-Sync-Cursor`, `X-Sync-Full`, `X-Sync-Deleted-Categories`; responds with `X-Sync-Cursor`, `X-Sync-Changed`, `X-Sync-Full-Requested`) |
| `GET /sync/v2/snapshot` | the whole merged library; `304` when `X-Sync-Cursor` is current |

Errors: `400` missing device id / invalid cursor / body not a backup, `401`, `413`, `404` (snapshot, nothing stored).

## Sync v1 (clients, deprecated)

Still supported; every response carries `Deprecation: true`. The payload is now rendered from
the server's item store and uploads are merged rather than stored as-is.

### `GET /sync/content`

Download the payload stored for the key.

| Header | Direction | Meaning |
|---|---|---|
| `If-None-Match` | request | ETag from a previous response; answer is `304` if unchanged |
| `X-Device-ID`, `X-Device-Name` | request | optional; records the device in the web UI |
| `ETag` | response | opaque version tag, echo it back verbatim |
| `Content-Encoding: gzip` | response | when the request sent `Accept-Encoding: gzip` |

Responses: `200` payload (`application/octet-stream`), `304`, `401`, `404` nothing stored yet.

### `PUT /sync/content`

Replace the payload and get a new `ETag`.

| Header | Meaning |
|---|---|
| `If-Match` | last ETag the client saw; `412` if the stored ETag differs (another device wrote in between). Omit to overwrite unconditionally. |
| `Content-Encoding: gzip` | body is gzip-compressed |
| `X-Device-ID`, `X-Device-Name` | optional device identification |

Responses: `200` + `ETag`, `400` body is not a valid backup, `401`, `412`, `413` body larger than `syncMaxBodySizeMB`.

### `POST /sync/event`

Report sync progress; drives notifications and the status shown in the web UI.

```json
{ "event": "SYNC_SUCCESS", "device_id": "…", "device_name": "Pixel 8", "message": "" }
```

`event` is one of `SYNC_STARTED`, `SYNC_SUCCESS`, `SYNC_FAILED`, `SYNC_ERROR`, `SYNC_CANCELLED`. `device_id`/`device_name` are optional; the `X-Device-*` headers take precedence. Responses: `204`, `400`, `401`.

### ETag semantics

- Opaque string, currently `seq=<n>` (`uuid=<uuid4>` before 1.3). Do not parse or compare for ordering.
- Sent and echoed **unquoted**; the value is the whole header.
- A restore from history (see below) produces a new ETag, so devices holding the old one get `412` on their next `If-Match` upload and re-sync.

## Sync admin (web UI, session only)

| Endpoint | Returns |
|---|---|
| `GET /sync/admin/{apikey}/status` | last upload, last event/status/device/message, stored payload size — `404` if nothing is known yet |
| `GET /sync/admin/{apikey}/devices` | devices seen for the key, newest first |
| `GET /sync/admin/{apikey}/history` | kept payload versions, newest first (`id`, `etag`, `size`, `created_at`) |
| `POST /sync/admin/{apikey}/history/{id}/restore` | rebuilds the item store from that version; returns `{"etag": "…"}`, `404` for an unknown id, `422` if the entry cannot be decoded |

## Other endpoints

| Endpoint | Purpose |
|---|---|
| `POST /auth/login`, `POST /auth/logout`, `GET /auth/validate` | web UI session |
| `GET /auth/onboard`, `POST /auth/onboard` | first-run admin user creation |
| `GET /keys`, `POST /keys`, `DELETE /keys/{apikey}` | API key management (deleting a key removes everything stored under it) |
| `GET /config`, `PATCH /config` | server configuration |
| `GET /notification`, `POST /notification`, `POST /notification/test`, `PUT /notification/{id}`, `DELETE /notification/{id}` | notification targets |
| `GET /logs/files`, `GET /logs/files/{file}` | log files |
| `GET /updates/latest`, `GET /updates/check` | release check |
| `GET /events?stream=logs` | server-sent log stream |
| `GET /healthz/liveness`, `GET /healthz/readiness` | health (unauthenticated) |
