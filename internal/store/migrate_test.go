package store_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/striver-21/portfolio-manager/internal/store"
	"github.com/striver-21/portfolio-manager/internal/store/storetest"
)

func TestMigrateIsIdempotent(t *testing.T) {
	db := storetest.Open(t) // already migrated once

	ran, err := store.Migrate(context.Background(), db, store.SQLiteMigrations())
	require.NoError(t, err)
	assert.Empty(t, ran, "second run must apply nothing")

	var count int64
	require.NoError(t, db.Table("schema_migrations").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestMigrateAppliesInOrderAndStopsOnError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:"), store.GormConfig())
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	fsys := fstest.MapFS{
		"0002_second.sql": {Data: []byte("INSERT INTO t (v) VALUES (2);")},
		"0001_first.sql":  {Data: []byte("CREATE TABLE t (v INTEGER);")},
		"0003_broken.sql": {Data: []byte("THIS IS NOT SQL;")},
	}
	ran, err := store.Migrate(context.Background(), db, fsys)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "0003_broken.sql")
	assert.Equal(t, []string{"0001_first.sql", "0002_second.sql"}, ran)

	var v int
	require.NoError(t, db.Raw("SELECT v FROM t").Scan(&v).Error)
	assert.Equal(t, 2, v)
}

func TestMigrateRejectsBadFileNames(t *testing.T) {
	db := storetest.Open(t)
	_, err := store.Migrate(context.Background(), db, fstest.MapFS{"init.sql": {Data: []byte("SELECT 1;")}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NNNN_name.sql")
}

func TestEmbeddedMigrationSetsMatch(t *testing.T) {
	// The Postgres and SQLite directories must stay in lock-step: same versions.
	pg, err := listNames(store.PostgresMigrations())
	require.NoError(t, err)
	lite, err := listNames(store.SQLiteMigrations())
	require.NoError(t, err)
	assert.Equal(t, pg, lite)
	assert.NotEmpty(t, pg)
}
