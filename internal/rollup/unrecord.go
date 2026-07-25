package rollup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"Canto/internal/db"
)

// unrecordQueueSize bounds how many pending listen deletions Unrecorder buffers before Enqueue blocks.
const unrecordQueueSize = 1000

// Unrecorder reverses listen deletions off the request path, one at a time and in submission order, so two deletes touching the same rows never race each other.
type Unrecorder struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	jobs    chan UnrecordedListen
}

// NewUnrecorder builds an Unrecorder backed by pool/queries.
func NewUnrecorder(pool *pgxpool.Pool, queries *db.Queries) *Unrecorder {
	return &Unrecorder{pool: pool, queries: queries, jobs: make(chan UnrecordedListen, unrecordQueueSize)}
}

// Enqueue queues evt for reversal, blocking if the queue is full rather than dropping it.
func (u *Unrecorder) Enqueue(evt UnrecordedListen) {
	u.jobs <- evt
}

// Run drains queued reversals one at a time until ctx is canceled, finishing whatever remains queued before returning.
func (u *Unrecorder) Run(ctx context.Context) {
	for {
		select {
		case evt := <-u.jobs:
			u.process(ctx, evt)
		case <-ctx.Done():
			for {
				select {
				case evt := <-u.jobs:
					u.process(context.Background(), evt)
				default:
					return
				}
			}
		}
	}
}

// process reverses one evt in its own transaction, logging and moving on on failure.
func (u *Unrecorder) process(ctx context.Context, evt UnrecordedListen) {
	tx, err := u.pool.Begin(ctx)
	if err != nil {
		slog.Error("rollup: begin unrecord tx failed", "user", evt.UserID, "song", evt.SongID, "err", err)
		return
	}
	defer tx.Rollback(ctx)

	if err := Unrecord(ctx, u.queries.WithTx(tx), evt); err != nil {
		slog.Error("rollup: unrecord failed", "user", evt.UserID, "song", evt.SongID, "err", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("rollup: commit unrecord tx failed", "user", evt.UserID, "song", evt.SongID, "err", err)
	}
}

// UnrecordedListen is an already-flushed listen being deleted, carrying what Unrecord needs to reverse it.
type UnrecordedListen struct {
	UserID         int64
	SongID         int64
	AlbumID        *int64
	ArtistIDs      []int64
	ListenedAt     time.Time
	PlayedMs       int32
	SongDurationMs int32
}

// minEligibleAtFunc returns the earliest remaining eligible listened_at for one entity, scoped to userID when given, or nil for every user.
type minEligibleAtFunc func(ctx context.Context, userID *int64) (pgtype.Timestamptz, error)

// Unrecord reverses evt's contribution to every rollup table, if it was eligible, then reconciles the user's streak state; q must be tx-scoped so a mid-way failure can't apply half the reversal.
func Unrecord(ctx context.Context, q *db.Queries, evt UnrecordedListen) error {
	if !Eligible(evt.PlayedMs, evt.SongDurationMs) {
		return nil
	}

	day := civilDay(evt.ListenedAt)
	hour := int16(evt.ListenedAt.UTC().Hour())
	playedMs := evt.PlayedMs
	if playedMs == 0 {
		playedMs = evt.SongDurationMs
	}

	if err := q.DecrementDailySongListens(ctx, db.DecrementDailySongListensParams{
		UserID: evt.UserID, SongID: evt.SongID, Day: pgtype.Date{Time: day, Valid: true}, PlayedMs: int64(playedMs),
	}); err != nil {
		return fmt.Errorf("rollup: decrement daily song listens: %w", err)
	}
	if err := q.PruneEmptyDailySongListens(ctx, db.PruneEmptyDailySongListensParams{
		UserID: evt.UserID, SongID: evt.SongID, Day: pgtype.Date{Time: day, Valid: true},
	}); err != nil {
		return fmt.Errorf("rollup: prune daily song listens: %w", err)
	}
	if err := q.DecrementClockCell(ctx, db.DecrementClockCellParams{UserID: evt.UserID, Day: pgtype.Date{Time: day, Valid: true}, Hour: hour}); err != nil {
		return fmt.Errorf("rollup: decrement clock cell: %w", err)
	}
	if err := q.PruneEmptyClockCell(ctx, db.PruneEmptyClockCellParams{UserID: evt.UserID, Day: pgtype.Date{Time: day, Valid: true}, Hour: hour}); err != nil {
		return fmt.Errorf("rollup: prune clock cell: %w", err)
	}

	remainingSong, err := q.RemainingUserSongPlays(ctx, db.RemainingUserSongPlaysParams{UserID: evt.UserID, SongID: evt.SongID})
	if err != nil {
		return fmt.Errorf("rollup: remaining user song plays: %w", err)
	}
	songMin := func(ctx context.Context, userID *int64) (pgtype.Timestamptz, error) {
		return q.MinEligibleListenedAtForSong(ctx, db.MinEligibleListenedAtForSongParams{SongID: evt.SongID, UserID: userID})
	}
	if err := applyEntityDecrement(ctx, q, db.EntityTypeSong, evt.SongID, evt.UserID, evt.ListenedAt, int64(playedMs), remainingSong, songMin); err != nil {
		return fmt.Errorf("rollup: unrecord song entity: %w", err)
	}

	if evt.AlbumID != nil {
		remainingAlbum, err := q.RemainingUserAlbumPlays(ctx, db.RemainingUserAlbumPlaysParams{UserID: evt.UserID, AlbumID: *evt.AlbumID})
		if err != nil {
			return fmt.Errorf("rollup: remaining user album plays: %w", err)
		}
		albumMin := func(ctx context.Context, userID *int64) (pgtype.Timestamptz, error) {
			return q.MinEligibleListenedAtForAlbum(ctx, db.MinEligibleListenedAtForAlbumParams{AlbumID: *evt.AlbumID, UserID: userID})
		}
		if err := applyEntityDecrement(ctx, q, db.EntityTypeAlbum, *evt.AlbumID, evt.UserID, evt.ListenedAt, int64(playedMs), remainingAlbum, albumMin); err != nil {
			return fmt.Errorf("rollup: unrecord album entity: %w", err)
		}
	}

	for _, artistID := range evt.ArtistIDs {
		remainingArtist, err := q.RemainingUserArtistPlays(ctx, db.RemainingUserArtistPlaysParams{UserID: evt.UserID, ArtistID: artistID})
		if err != nil {
			return fmt.Errorf("rollup: remaining user artist plays: %w", err)
		}
		artistMin := func(ctx context.Context, userID *int64) (pgtype.Timestamptz, error) {
			return q.MinEligibleListenedAtForArtist(ctx, db.MinEligibleListenedAtForArtistParams{ArtistID: artistID, UserID: userID})
		}
		if err := applyEntityDecrement(ctx, q, db.EntityTypeArtist, artistID, evt.UserID, evt.ListenedAt, int64(playedMs), remainingArtist, artistMin); err != nil {
			return fmt.Errorf("rollup: unrecord artist entity: %w", err)
		}
	}

	return ReconcileUserState(ctx, q, evt.UserID)
}

// applyEntityDecrement reverses one eligible listen's contribution to entity_global_stats and user_entity_first_listen for one (entityType, entityID).
func applyEntityDecrement(ctx context.Context, q *db.Queries, entityType db.EntityType, entityID, userID int64, deletedAt time.Time, playedMs int64, remainingForUser int64, minAt minEligibleAtFunc) error {
	if err := q.DecrementEntityGlobalPlays(ctx, db.DecrementEntityGlobalPlaysParams{EntityType: entityType, EntityID: entityID, PlayedMs: playedMs}); err != nil {
		return err
	}

	if remainingForUser == 0 {
		if err := q.DecrementEntityGlobalUniqueListeners(ctx, db.DecrementEntityGlobalUniqueListenersParams{EntityType: entityType, EntityID: entityID}); err != nil {
			return err
		}
		if err := q.DeleteFirstListenForUserEntity(ctx, db.DeleteFirstListenForUserEntityParams{UserID: userID, EntityType: entityType, EntityID: entityID}); err != nil {
			return err
		}
	} else if err := fixFirstListenForUserEntity(ctx, q, entityType, entityID, userID, deletedAt, minAt); err != nil {
		return err
	}

	if err := q.PruneEmptyEntityGlobalStats(ctx, db.PruneEmptyEntityGlobalStatsParams{EntityType: entityType, EntityID: entityID}); err != nil {
		return err
	}

	globalFirst, err := q.GetEntityGlobalFirstListenedAt(ctx, db.GetEntityGlobalFirstListenedAtParams{EntityType: entityType, EntityID: entityID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // pruned: no listens of this entity remain, nothing left to fix
	}
	if err != nil {
		return err
	}
	if !globalFirst.Valid || !globalFirst.Time.Equal(deletedAt) {
		return nil
	}
	newFirst, err := minAt(ctx, nil)
	if err != nil {
		return err
	}
	return q.SetEntityGlobalFirstListenedAt(ctx, db.SetEntityGlobalFirstListenedAtParams{EntityType: entityType, EntityID: entityID, FirstListenedAt: newFirst})
}

// fixFirstListenForUserEntity re-derives (userID, entityType, entityID)'s first_at if deletedAt was the recorded value.
func fixFirstListenForUserEntity(ctx context.Context, q *db.Queries, entityType db.EntityType, entityID, userID int64, deletedAt time.Time, minAt minEligibleAtFunc) error {
	current, err := q.GetFirstListenForUserEntity(ctx, db.GetFirstListenForUserEntityParams{UserID: userID, EntityType: entityType, EntityID: entityID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !current.Time.Equal(deletedAt) {
		return nil
	}
	newFirst, err := minAt(ctx, &userID)
	if err != nil {
		return err
	}
	return q.SetFirstListenForUserEntity(ctx, db.SetFirstListenForUserEntityParams{UserID: userID, EntityType: entityType, EntityID: entityID, FirstAt: newFirst})
}
