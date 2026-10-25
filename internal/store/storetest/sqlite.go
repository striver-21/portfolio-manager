// Package storetest opens throwaway in-memory SQLite databases for tests. It is
// a separate package so the production binary never links the SQLite driver.
package storetest

import (
	"context"
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/striver-21/portfolio-manager/internal/store"
)

// Open returns a migrated, private in-memory database that is closed when the
// test ends. Each call gets its own database, so tests cannot leak state into
// one another, and the pool is pinned to one connection because every SQLite
// connection to a memory DSN is otherwise a different database.
func Open(t testing.TB) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(1)", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), store.GormConfig())
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if _, err := store.Migrate(context.Background(), db, store.SQLiteMigrations()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}
