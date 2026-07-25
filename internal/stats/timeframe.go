package stats

import (
	"fmt"
	"time"
)

// Timeframe is either an explicit From/To range, a calendar Year/Month/Week, or a relative rolling Period.
type Timeframe struct {
	Period string `json:"period,omitempty"` // day|week|month|year|all_time
	From   *int64 `json:"from,omitempty"`   // unix seconds
	To     *int64 `json:"to,omitempty"`     // unix seconds
	Year   *int   `json:"year,omitempty"`
	Month  *int   `json:"month,omitempty"` // 1-12, requires Year
	Week   *int   `json:"week,omitempty"`  // ISO week, requires Year
	TZ     string `json:"tz,omitempty"`
}

// TZOrUTC returns tf.TZ, defaulting to "UTC" when unset.
func (tf Timeframe) TZOrUTC() string {
	if tf.TZ == "" {
		return "UTC"
	}
	return tf.TZ
}

// Resolve turns tf into concrete [from, to) bounds relative to now, in tf's timezone (UTC if unset).
func (tf Timeframe) Resolve(now time.Time) (from, to time.Time, err error) {
	loc := time.UTC
	if tf.TZ != "" {
		loc, err = time.LoadLocation(tf.TZ)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("stats: invalid tz %q: %w", tf.TZ, err)
		}
	}
	now = now.In(loc)

	switch {
	case tf.From != nil && tf.To != nil:
		return time.Unix(*tf.From, 0).In(loc), time.Unix(*tf.To, 0).In(loc), nil
	case tf.Year != nil && tf.Month != nil:
		start := time.Date(*tf.Year, time.Month(*tf.Month), 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(0, 1, 0), nil
	case tf.Year != nil && tf.Week != nil:
		start := isoWeekStart(*tf.Year, *tf.Week, loc)
		return start, start.AddDate(0, 0, 7), nil
	case tf.Year != nil:
		start := time.Date(*tf.Year, 1, 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(1, 0, 0), nil
	}

	switch tf.Period {
	case "", "all_time":
		return time.Unix(0, 0).In(loc), now.AddDate(0, 0, 1), nil
	case "day":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		return start, start.AddDate(0, 0, 1), nil
	case "week":
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		start := day.AddDate(0, 0, -int(day.Weekday()-time.Monday+7)%7)
		return start, start.AddDate(0, 0, 7), nil
	case "month":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(0, 1, 0), nil
	case "year":
		start := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(1, 0, 0), nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("stats: invalid period %q", tf.Period)
	}
}

// isoWeekStart returns the Monday of ISO week "week" in "year", in loc.
func isoWeekStart(year, week int, loc *time.Location) time.Time {
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, loc)
	firstMonday := jan4.AddDate(0, 0, -int(jan4.Weekday()-time.Monday+7)%7)
	return firstMonday.AddDate(0, 0, (week-1)*7)
}
