//go:build integration

// Package testhelper provides a shared test harness for integration tests.
//
// Usage in a test file:
//
//	//go:build integration
//
//	func TestFoo(t *testing.T) {
//	    pool := testhelper.StartTestDB(t)
//	    defer testhelper.TruncateAll(t, pool)
//	    // ... use pool
//	}
package testhelper

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// migrationsDir returns an absolute path to the migrations/ directory
// regardless of which package's test file calls this.
func migrationsDir() string {
	// __file__ of this source file is internal/testhelper/db.go.
	// Migrations live two directories up at the repo root.
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
}

// StartTestDB starts a PostgreSQL 17 container, runs all goose migrations,
// and returns a ready-to-use *pgxpool.Pool.
//
// The container is automatically terminated when the test (or sub-test) ends
// via t.Cleanup. Each call produces a fully isolated database.
func StartTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:17-alpine",
		postgres.WithDatabase("vmarble_test"),
		postgres.WithUsername("vmarble"),
		postgres.WithPassword("vmarble"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("warn: terminate postgres container: %v", err)
		}
	})

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	// Run migrations via goose using the stdlib driver.
	if err := runMigrations(dsn); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	// Build pgx connection pool for test use.
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("create pgx pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping db: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

func runMigrations(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open sql connection: %w", err)
	}
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.Up(db, migrationsDir()); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}

// TruncateAll deletes all application rows in dependency-safe order so tests
// start with a clean state without re-creating the container.
func TruncateAll(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// Order matters: child tables before parents (foreign key constraints).
	tables := []string{
		"scan_events",
		"barcodes",
		"costing_records",
		"consumption_records",
		"cutting_records",
		"remnants",
		"board_sheets",
		"inventory_lots",
		"work_orders",
		"plan_items",
		"production_plans",
		"po_line_items",
		"purchase_orders",
		"bom_components",
		"skus",
		"materials",
	}

	for _, tbl := range tables {
		if _, err := pool.Exec(ctx, "DELETE FROM "+tbl); err != nil {
			t.Logf("warn: truncate %s: %v", tbl, err)
		}
	}
}
