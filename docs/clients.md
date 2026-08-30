# Client guide

What a client (TachiyomiSY, Komikku, Suwayomi-Server, …) has to do to use SyncYomi, and how
the combinations behave.

## Compatibility

| Client                                | Server < 1.3                                | Server ≥ 1.3                         |
| ------------------------------------- | ------------------------------------------- | ------------------------------------ |
| v1 client (`GET`/`PUT /sync/content`) | works, client-side merge                    | works, server-side merge, deprecated |
| v2 client                             | falls back to v1 (capabilities returns 404) | v2                                   |

A library can be shared by v1 and v2 clients at the same time.

## Implementing v2

1. **Probe once**: `GET /api/sync/v2/capabilities` with `X-API-Token`. 200 → use v2; 404 →
   keep using v1. Re-probe when the host changes.
2. **Identify the device**: generate a UUID once and send it as `X-Device-ID` on every
   request; send a human-readable `X-Device-Name` too.
3. **Build the upload**: the same backup you already produce for v1, restricted to
   - manga modified since your last successful upload (their own `last_modified_at` or
     `version` — your own clock is reliable for your own writes), each with only the chapters
     modified since then,
   - all categories,
   - settings sections only when they changed (or always; they are small).

   Send the whole library with `X-Sync-Full: true` on the first sync, when your cursor is 0,
   when the last response carried `X-Sync-Full-Requested`, and occasionally (once a day) as a
   safety net.

4. **Track deletions**: when the user deletes a category, remember its `uid` and send the
   pending uids in `X-Sync-Deleted-Categories`; clear them after a 200.
5. **`POST /api/sync/v2/merge`** with `X-Sync-Cursor` = the cursor from the last response.
   Do **not** run a local merge on the response.
6. **Apply the response**: if `X-Sync-Changed` is `false`, record success and stop. Otherwise
   restore the response with your normal upsert restore, delete local categories that are
   absent from the response's category list, and store the new `X-Sync-Cursor`.
7. **Report** `POST /api/sync/event` as before.

Keep the v1 code path for old servers; it is unchanged.

Give every scalar field of our backup models a default (`0`, `""`, `false`): protobuf
omits zero values, and other clients (Suwayomi) never send them, so a model that requires
them fails to decode a library that originated elsewhere.

### TachiyomiSY specifics

- `SyncPreferences`: add `syncCursor` (Long), `serverSupportsV2` (unknown/yes/no, reset when
  the host changes), `pendingDeletedCategoryUids` (String set), `lastPushedAt` (Long).
- Category deletion (`DeleteCategory` interactor): add the `uid` to
  `pendingDeletedCategoryUids` when sync is enabled.
- `SyncYomiSyncService.doSync`: probe, then either the existing v1 flow or the v2 request; in
  v2 skip `mergeSyncData`.
- `SyncManager.syncData`: on v2 skip the "first sync, don't restore" branch and the "no data
  found on remote server" check; skip the restore when `X-Sync-Changed` is `false`; keep
  `filterFavoritesAndNonFavorites`, `updateNonFavorites` and the category delete-by-absence.
- Send `X-Device-ID`/`X-Device-Name` on v1 requests as well; the web UI then shows the device.

### Suwayomi-Server specifics

Same shape as TachiyomiSY: `global/impl/sync/SyncYomiSyncService.kt` (probe, v2 request,
skip `mergeSyncData`), `global/impl/sync/SyncManager.kt` (skip restore when unchanged, drop
the first-sync branch), `Category.removeCategory` (remember the uid), preferences in the
`sync` `SharedPreferences` (`sync_cursor`, `server_supports_v2`,
`pending_deleted_category_uids`, `device_id`, `last_pushed_at`).
