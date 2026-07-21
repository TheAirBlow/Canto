// Package rollup incrementally aggregates listens into small precomputed stats tables.
package rollup

// minEligibleMs is the play-duration floor a listen qualifies at regardless of song length.
const minEligibleMs = 4 * 60 * 1000

// Eligible reports whether a listen counts toward stats: at least half the song's duration, or at least 4 minutes.
func Eligible(playedMs, durationMs int32) bool {
	if durationMs <= 0 {
		return true
	}
	return playedMs*2 >= durationMs || playedMs >= minEligibleMs
}
