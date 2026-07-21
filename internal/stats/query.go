package stats

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ts converts t into a pgtype.Timestamptz query param.
func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// day converts t into a pgtype.Date query param, truncating to its UTC calendar day.
func day(t time.Time) pgtype.Date {
	t = t.UTC()
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

// stepIntervals maps Activity/Discovery's step param to a Postgres interval.
var stepIntervals = map[string]pgtype.Interval{
	"day":   {Days: 1, Valid: true},
	"week":  {Days: 7, Valid: true},
	"month": {Months: 1, Valid: true},
	"year":  {Months: 12, Valid: true},
}

// ratio divides a/b, returning 0 instead of NaN/Inf when b is 0.
func ratio(a, b int64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}
