package httpkit

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// safeTableName is intentionally restrictive. EstimateRowCount interpolates
// the table name into a regclass cast (`'%s'::regclass`), and pgx cannot
// parameterise identifiers in that position. Whitelisting `[a-z][a-z0-9_]*`
// keeps callers from accidentally passing user input through to a query
// where it could be exploited.
var safeTableName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// EstimateRowCount returns the autovacuum stats collector's estimate of live
// rows for table. It reads pg_stat_user_tables.n_live_tup rather than
// running COUNT(*), which on a 10M-row table is a multi-second sequential
// scan even on warm cache.
//
// Trade-off: n_live_tup is updated by the autovacuum stats collector, so it
// lags reality by anywhere from seconds (heavy write load → frequent
// autovacuum) to days (read-heavy table that autovacuum rarely visits).
// Sufficient for "show ~1.2M rows" UI affordances and dashboard counters;
// not for invariants or anything financially sensitive.
//
// Callers should display the returned number with a leading "~" or pair it
// with a `total_is_estimate: true` flag so users know not to trust the last
// few digits.
//
// Returns 0 (not an error) when the table is missing from
// pg_stat_user_tables — that is the legitimate "fresh table, never had a
// stats sample" state. Returns an error when the table name does not match
// the safe-identifier pattern or the query itself fails.
func EstimateRowCount(ctx context.Context, pool *pgxpool.Pool, table string) (int64, error) {
	if !safeTableName.MatchString(table) {
		return 0, fmt.Errorf("estimate row count: invalid table name %q", table)
	}
	// schemaname='public' is hard-coded because the application only owns
	// public-schema tables. Cross-schema queries should not paginate via
	// estimates.
	const q = `SELECT COALESCE(n_live_tup, 0)
	           FROM pg_stat_user_tables
	           WHERE schemaname = 'public' AND relname = $1`

	var n int64
	if err := pool.QueryRow(ctx, q, table).Scan(&n); err != nil {
		// pgx.ErrNoRows is the "stats collector hasn't seen this table"
		// case; treat as 0 rather than an error.
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("estimate row count for %q: %w", table, err)
	}
	return n, nil
}
