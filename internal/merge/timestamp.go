package merge

// EpochMilliThreshold is the smallest value treated as Unix milliseconds. It is 1e11,
// which is year 5138 in seconds, so no real seconds timestamp reaches it while every
// millisecond timestamp after 1973 does.
const EpochMilliThreshold int64 = 100_000_000_000

// NormalizeTimestamp returns ts in Unix seconds, the single canonical unit of
// sync_item.modified_at. Values at or above EpochMilliThreshold are epoch milliseconds
// (BackupHistory.last_read is java.util.Date.time, i.e. milliseconds) and are divided by
// 1000; everything else is already seconds and returned unchanged, so the call is a no-op
// for seconds-based manga/chapter/category timestamps and for tombstone stamps.
func NormalizeTimestamp(ts int64) int64 {
	if ts >= EpochMilliThreshold {
		return ts / 1000
	}
	return ts
}
