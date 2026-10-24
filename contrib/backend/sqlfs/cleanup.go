package sqlfs

import (
	"context"
	"fmt"
)

// CleanupStats reports what Cleanup removed.
type CleanupStats struct {
	// StagedRemoved counts abandoned staged revisions removed.
	StagedRemoved int
	// SupersededRemoved counts replaced or deleted revisions removed after their retention period.
	SupersededRemoved int
}

// Cleanup removes staged writes that were abandoned (idle longer than the staging retention) and content that was
// replaced or deleted longer ago than the revision retention. Current published content is never removed. Run it on
// a schedule of your choosing; the backend starts no background work. Cleanup stops and returns the context's error
// if ctx is canceled.
func (fsys *FileSystem) Cleanup(ctx context.Context) (CleanupStats, error) {
	var stats CleanupStats
	for {
		if err := ctx.Err(); err != nil {
			return stats, fmt.Errorf("sqlfs: cleanup: %w", err)
		}
		batch, err := fsys.cleanupCandidates(ctx)
		if err != nil {
			return stats, fsys.wrapErr("cleanup", err)
		}
		if len(batch) == 0 {
			return stats, nil
		}
		for _, c := range batch {
			if err := ctx.Err(); err != nil {
				return stats, fmt.Errorf("sqlfs: cleanup: %w", err)
			}
			removed, err := fsys.reclaim(ctx, c)
			if err != nil {
				return stats, err
			}
			switch {
			case !removed:
			case c.state == stateStaged:
				stats.StagedRemoved++
			default:
				stats.SupersededRemoved++
			}
		}
	}
}

// reclaim removes one candidate revision in its own transaction, so a large revision neither holds locks on others
// nor makes a whole batch roll back. It reports false if the revision was no longer eligible.
func (fsys *FileSystem) reclaim(ctx context.Context, c cleanupCandidate) (bool, error) {
	var removed bool
	err := fsys.inTx(ctx, "cleanup", func(ctx context.Context, tx txn) error {
		removed = false
		// Claiming rechecks eligibility atomically and locks the revision row, so a writer that renewed or published
		// it after selection makes the claim a no-op, and a writer that tries afterwards waits and then finds the
		// revision reclaimed.
		now := nowNanos()
		claimed, err := execRows(ctx, tx, fsys.qs.claimRevision, c.id, now, now-fsys.revisionRetention.Nanoseconds())
		if err != nil || claimed == 0 {
			return err
		}
		if err := fsys.deleteRevision(ctx, tx, c.id, c.chunkSize); err != nil {
			return err
		}
		removed = true
		return nil
	})
	return removed, err
}

type cleanupCandidate struct {
	id        string
	state     int
	chunkSize int
}

func (fsys *FileSystem) cleanupCandidates(ctx context.Context) ([]cleanupCandidate, error) {
	ctx, cancel := fsys.opCtx(ctx)
	defer cancel()
	now := nowNanos()
	rows, err := fsys.q.QueryContext(ctx, fsys.qs.selCleanup, now, now-fsys.revisionRetention.Nanoseconds(), cleanupBatch)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []cleanupCandidate
	for rows.Next() {
		var c cleanupCandidate
		if err := rows.Scan(&c.id, &c.state, &c.chunkSize); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
