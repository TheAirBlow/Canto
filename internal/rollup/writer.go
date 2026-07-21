package rollup

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"Canto/internal/db"
)

// defaultFlushInterval is used when NewWriter is given a non-positive interval.
const defaultFlushInterval = time.Second

// maxBufferedEvents triggers an out-of-cycle flush, bounding memory during a fast bulk import.
const maxBufferedEvents = 5000

// ListenEvent is one listen for the Writer to fold into its buffers, once eligible.
type ListenEvent struct {
	UserID         int64
	SongID         int64
	ArtistIDs      []int64
	AlbumID        *int64
	ListenedAt     time.Time
	PlayedMs       int32
	SongDurationMs int32
	Imported       bool // came through the importer: historical/out-of-order, skips live streak updates
}

// civilDay truncates t to its UTC calendar day, as a comparable/steppable instant.
func civilDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// bufferedEvent is ListenEvent reduced to what flush needs, already past the eligibility check.
type bufferedEvent struct {
	userID     int64
	songID     int64
	artistIDs  []int64
	albumID    *int64
	listenedAt time.Time
	minutesMs  int64
	imported   bool
}

// Writer buffers listen deltas in memory and flushes them to Postgres in batched upserts.
type Writer struct {
	queries       *db.Queries
	flushInterval time.Duration
	flushSignal   chan struct{}

	mu     sync.Mutex
	events []bufferedEvent
}

// NewWriter builds a Writer that flushes at flushInterval (or defaultFlushInterval if <= 0).
func NewWriter(queries *db.Queries, flushInterval time.Duration) *Writer {
	if flushInterval <= 0 {
		flushInterval = defaultFlushInterval
	}
	return &Writer{queries: queries, flushInterval: flushInterval, flushSignal: make(chan struct{}, 1)}
}

// Record buffers evt for the next flush if it's eligible; it never touches the database.
func (w *Writer) Record(evt ListenEvent) {
	if !Eligible(evt.PlayedMs, evt.SongDurationMs) {
		return
	}
	minutesMs := evt.PlayedMs
	if minutesMs == 0 {
		minutesMs = evt.SongDurationMs
	}

	w.mu.Lock()
	w.events = append(w.events, bufferedEvent{
		userID: evt.UserID, songID: evt.SongID, artistIDs: evt.ArtistIDs, albumID: evt.AlbumID,
		listenedAt: evt.ListenedAt, minutesMs: int64(minutesMs), imported: evt.Imported,
	})
	n := len(w.events)
	w.mu.Unlock()

	if n >= maxBufferedEvents {
		select {
		case w.flushSignal <- struct{}{}:
		default:
		}
	}
}

// Run flushes on a timer and on buffer overflow until ctx is canceled, flushing once more before returning.
func (w *Writer) Run(ctx context.Context) {
	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.flush(context.Background())
			return
		case <-ticker.C:
			w.flush(ctx)
		case <-w.flushSignal:
			w.flush(ctx)
		}
	}
}

type dailyKey struct {
	userID int64
	songID int64
	day    time.Time
}

type dailyDelta struct {
	count     int32
	minutesMs int64
}

type clockKey struct {
	userID int64
	day    time.Time
	hour   int16
}

type firstSeenKey struct {
	userID     int64
	entityType db.EntityType
	entityID   int64
}

type entityKey struct {
	entityType db.EntityType
	entityID   int64
}

type entityDelta struct {
	plays   int32
	firstAt time.Time
}

// flush snapshots the buffer, drops blacklisted (user, song) pairs, aggregates the rest, and issues one batched upsert per table.
func (w *Writer) flush(ctx context.Context) {
	w.mu.Lock()
	if len(w.events) == 0 {
		w.mu.Unlock()
		return
	}
	events := w.events
	w.events = nil
	w.mu.Unlock()

	blacklisted := w.blacklistedPairs(ctx, events)

	daily := make(map[dailyKey]*dailyDelta)
	clock := make(map[clockKey]int32)
	firstSeen := make(map[firstSeenKey]time.Time)
	entities := make(map[entityKey]*entityDelta)
	liveByUser := make(map[int64][]time.Time)

	touchFirstSeen := func(k firstSeenKey, at time.Time) {
		if cur, ok := firstSeen[k]; !ok || at.Before(cur) {
			firstSeen[k] = at
		}
	}
	touchEntity := func(k entityKey, at time.Time) {
		e := entities[k]
		if e == nil {
			entities[k] = &entityDelta{plays: 1, firstAt: at}
			return
		}
		e.plays++
		if at.Before(e.firstAt) {
			e.firstAt = at
		}
	}

	for _, e := range events {
		if blacklisted[[2]int64{e.userID, e.songID}] {
			continue // a later un-blacklist won't retroactively restore rows skipped here
		}

		day := civilDay(e.listenedAt)
		dk := dailyKey{e.userID, e.songID, day}
		d := daily[dk]
		if d == nil {
			d = &dailyDelta{}
			daily[dk] = d
		}
		d.count++
		d.minutesMs += e.minutesMs

		clock[clockKey{e.userID, day, int16(e.listenedAt.UTC().Hour())}]++

		touchFirstSeen(firstSeenKey{e.userID, db.EntityTypeSong, e.songID}, e.listenedAt)
		touchEntity(entityKey{db.EntityTypeSong, e.songID}, e.listenedAt)
		if e.albumID != nil {
			touchFirstSeen(firstSeenKey{e.userID, db.EntityTypeAlbum, *e.albumID}, e.listenedAt)
			touchEntity(entityKey{db.EntityTypeAlbum, *e.albumID}, e.listenedAt)
		}
		for _, artistID := range e.artistIDs {
			touchFirstSeen(firstSeenKey{e.userID, db.EntityTypeArtist, artistID}, e.listenedAt)
			touchEntity(entityKey{db.EntityTypeArtist, artistID}, e.listenedAt)
		}

		if !e.imported {
			liveByUser[e.userID] = append(liveByUser[e.userID], e.listenedAt)
		}
	}

	w.flushDaily(ctx, daily)
	w.flushClock(ctx, clock)
	w.flushFirstSeen(ctx, firstSeen)
	w.flushEntityPlays(ctx, entities)
	if len(liveByUser) > 0 {
		w.reconcileLiveStates(ctx, liveByUser)
	}
}

// blacklistedPairs returns which (user_id, song_id) pairs touched by events are currently blacklisted.
func (w *Writer) blacklistedPairs(ctx context.Context, events []bufferedEvent) map[[2]int64]bool {
	userSet := make(map[int64]struct{})
	songSet := make(map[int64]struct{})
	for _, e := range events {
		userSet[e.userID] = struct{}{}
		songSet[e.songID] = struct{}{}
	}
	userIDs := make([]int64, 0, len(userSet))
	for id := range userSet {
		userIDs = append(userIDs, id)
	}
	songIDs := make([]int64, 0, len(songSet))
	for id := range songSet {
		songIDs = append(songIDs, id)
	}

	rows, err := w.queries.CheckBlacklistedSongs(ctx, db.CheckBlacklistedSongsParams{UserIds: userIDs, SongIds: songIDs})
	if err != nil {
		slog.Error("rollup: check blacklisted songs failed", "err", err)
		return nil
	}
	out := make(map[[2]int64]bool, len(rows))
	for _, r := range rows {
		out[[2]int64{r.UserID, r.SongID}] = true
	}
	return out
}

func (w *Writer) flushDaily(ctx context.Context, daily map[dailyKey]*dailyDelta) {
	if len(daily) == 0 {
		return
	}
	var p db.UpsertDailySongListensParams
	for k, d := range daily {
		p.UserIds = append(p.UserIds, k.userID)
		p.SongIds = append(p.SongIds, k.songID)
		p.Days = append(p.Days, pgtype.Date{Time: k.day, Valid: true})
		p.ListenCounts = append(p.ListenCounts, d.count)
		p.MinutesMs = append(p.MinutesMs, d.minutesMs)
	}
	if err := w.queries.UpsertDailySongListens(ctx, p); err != nil {
		slog.Error("rollup: upsert daily song listens failed", "err", err)
	}
}

func (w *Writer) flushClock(ctx context.Context, clock map[clockKey]int32) {
	if len(clock) == 0 {
		return
	}
	var p db.UpsertClockCellsParams
	for k, count := range clock {
		p.UserIds = append(p.UserIds, k.userID)
		p.Days = append(p.Days, pgtype.Date{Time: k.day, Valid: true})
		p.Hours = append(p.Hours, k.hour)
		p.ListenCounts = append(p.ListenCounts, count)
	}
	if err := w.queries.UpsertClockCells(ctx, p); err != nil {
		slog.Error("rollup: upsert clock cells failed", "err", err)
	}
}

// flushFirstSeen inserts first-sighting candidates and bumps unique_listeners for the genuinely new ones.
func (w *Writer) flushFirstSeen(ctx context.Context, firstSeen map[firstSeenKey]time.Time) {
	if len(firstSeen) == 0 {
		return
	}
	var p db.InsertFirstListensParams
	for k, at := range firstSeen {
		p.UserIds = append(p.UserIds, k.userID)
		p.EntityTypes = append(p.EntityTypes, string(k.entityType))
		p.EntityIds = append(p.EntityIds, k.entityID)
		p.FirstAts = append(p.FirstAts, pgtype.Timestamptz{Time: at, Valid: true})
	}
	newRows, err := w.queries.InsertFirstListens(ctx, p)
	if err != nil {
		slog.Error("rollup: insert first listens failed", "err", err)
		return
	}
	if len(newRows) == 0 {
		return
	}

	counts := make(map[entityKey]int32, len(newRows))
	for _, r := range newRows {
		counts[entityKey{r.EntityType, r.EntityID}]++
	}
	var bp db.BumpEntityGlobalUniqueListenersParams
	for k, c := range counts {
		bp.EntityTypes = append(bp.EntityTypes, string(k.entityType))
		bp.EntityIds = append(bp.EntityIds, k.entityID)
		bp.Counts = append(bp.Counts, c)
	}
	if err := w.queries.BumpEntityGlobalUniqueListeners(ctx, bp); err != nil {
		slog.Error("rollup: bump entity unique listeners failed", "err", err)
	}
}

func (w *Writer) flushEntityPlays(ctx context.Context, entities map[entityKey]*entityDelta) {
	if len(entities) == 0 {
		return
	}
	var p db.UpsertEntityGlobalPlaysParams
	for k, d := range entities {
		p.EntityTypes = append(p.EntityTypes, string(k.entityType))
		p.EntityIds = append(p.EntityIds, k.entityID)
		p.Plays = append(p.Plays, d.plays)
		p.FirstAts = append(p.FirstAts, pgtype.Timestamptz{Time: d.firstAt, Valid: true})
	}
	if err := w.queries.UpsertEntityGlobalPlays(ctx, p); err != nil {
		slog.Error("rollup: upsert entity global plays failed", "err", err)
	}
}

// reconcileLiveStates folds each user's buffered live listens, in order, into their current streak state.
func (w *Writer) reconcileLiveStates(ctx context.Context, liveByUser map[int64][]time.Time) {
	userIDs := make([]int64, 0, len(liveByUser))
	for uid := range liveByUser {
		userIDs = append(userIDs, uid)
	}

	states, err := w.queries.GetUserListenStates(ctx, userIDs)
	if err != nil {
		slog.Error("rollup: get user listen states failed", "err", err)
		return
	}
	byUser := make(map[int64]db.UserListenState, len(states))
	for _, s := range states {
		byUser[s.UserID] = s
	}

	var p db.UpsertUserListenStatesParams
	for uid, times := range liveByUser {
		sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })

		state := byUser[uid]
		current := state.CurrentStreak
		longest := state.LongestStreak
		var lastDay, lastAt time.Time
		if state.LastListenedAt.Valid {
			lastDay = civilDay(state.LastListenedAt.Time)
			lastAt = state.LastListenedAt.Time
		}

		for _, t := range times {
			day := civilDay(t)
			switch {
			case lastDay.IsZero():
				current = 1
			case day.Equal(lastDay):
				// same day, streak unchanged
			case day.Equal(lastDay.AddDate(0, 0, 1)):
				current++
			default:
				current = 1
			}
			if current > longest {
				longest = current
			}
			lastDay = day
			lastAt = t
		}

		p.UserIds = append(p.UserIds, uid)
		p.LastListenedAts = append(p.LastListenedAts, pgtype.Timestamptz{Time: lastAt, Valid: true})
		p.CurrentStreaks = append(p.CurrentStreaks, current)
		p.LongestStreaks = append(p.LongestStreaks, longest)
	}

	if err := w.queries.UpsertUserListenStates(ctx, p); err != nil {
		slog.Error("rollup: upsert user listen states failed", "err", err)
	}
}
