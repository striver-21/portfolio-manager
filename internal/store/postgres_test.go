package store_test

import (
	"context"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/striver-21/portfolio-manager/internal/store"
)

// TestPostgresMigrationsApply runs the real Postgres migrations against the
// database named by TEST_DATABASE_URL (CI provides one as a service container).
// It is skipped when the variable is unset so the suite needs no Docker locally.
func TestPostgresMigrationsApply(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := store.OpenPostgres(ctx, dsn)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	// Start from a clean slate so the test is repeatable on a reused database.
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS assets, portfolios, users, schema_migrations CASCADE").Error)

	ran, err := store.Migrate(ctx, db, store.PostgresMigrations())
	require.NoError(t, err)
	want, err := listNames(store.PostgresMigrations())
	require.NoError(t, err)
	require.Equal(t, want, ran)

	again, err := store.Migrate(ctx, db, store.PostgresMigrations())
	require.NoError(t, err)
	require.Empty(t, again)

	// Exercise the store against the real schema once.
	s := store.NewGorm(db)
	u := seedUser(t, s, "pg@example.com")
	p := seedPortfolio(t, s, u.ID, "PG")
	got, err := s.Portfolio(ctx, p.ID, u.ID)
	require.NoError(t, err)
	require.Equal(t, "PG", got.Name)
}

func listNames(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}
