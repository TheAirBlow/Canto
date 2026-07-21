package stats

import (
	"context"
	"encoding/json"
	"time"

	"Canto/internal/db"
)

// clockCell is one hour-of-day x day-of-week grid cell.
type clockCell struct {
	DayOfWeek   int   `json:"day_of_week"` // 0=Sunday .. 6=Saturday
	Hour        int   `json:"hour"`        // 0-23
	ListenCount int64 `json:"listen_count"`
}

// Clock computes userID's listening-clock heatmap (hour x weekday) for tf.
func (e *Engine) Clock(ctx context.Context, userID *int64, tf Timeframe) (json.RawMessage, error) {
	from, to, err := tf.Resolve(time.Now())
	if err != nil {
		return nil, err
	}
	key := cacheKey{UserID: userID, Resource: db.StatsResourceClock, Params: tf}
	return e.cached(ctx, key, func(ctx context.Context) (any, error) {
		return e.computeClock(ctx, userID, from, to)
	})
}

// computeClock runs RollupClockGrid and zero-fills every one of the 168 cells.
func (e *Engine) computeClock(ctx context.Context, userID *int64, from, to time.Time) ([]clockCell, error) {
	rows, err := e.queries.RollupClockGrid(ctx, db.RollupClockGridParams{UserID: userID, FromDay: day(from), ToDay: day(to)})
	if err != nil {
		return nil, err
	}
	counts := make(map[[2]int]int64, len(rows))
	for _, r := range rows {
		counts[[2]int{int(r.DayOfWeek), int(r.Hour)}] = r.ListenCount
	}

	cells := make([]clockCell, 0, 7*24)
	for dow := 0; dow < 7; dow++ {
		for hour := 0; hour < 24; hour++ {
			cells = append(cells, clockCell{DayOfWeek: dow, Hour: hour, ListenCount: counts[[2]int{dow, hour}]})
		}
	}
	return cells, nil
}
