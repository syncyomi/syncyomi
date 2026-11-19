package merge

const EpochMilliThreshold int64 = 100_000_000_000

// BackupHistory.last_read is java.util.Date.time, so it arrives in milliseconds while every other synced timestamp is in seconds.
func NormalizeTimestamp(ts int64) int64 {
	if ts >= EpochMilliThreshold {
		return ts / 1000
	}
	return ts
}
