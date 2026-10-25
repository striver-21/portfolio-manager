package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"time"

	"gorm.io/gorm"
)

// Migration files live next to this package and are compiled into the binary,
// so a deployed server carries exactly the schema it was built against.
//
//go:embed migrations/postgres/*.sql
var postgresMigrations embed.FS

//go:embed migrations/sqlite/*.sql
var sqliteMigrations embed.FS

// PostgresMigrations returns the Postgres schema migrations.
func PostgresMigrations() fs.FS { return mustSub(postgresMigrations, "migrations/postgres") }

// SQLiteMigrations returns the SQLite flavour used by the test suite.
func SQLiteMigrations() fs.FS { return mustSub(sqliteMigrations, "migrations/sqlite") }

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err) // the directory is embedded at compile time; cannot fail at runtime
	}
	return sub
}

// schemaMigration is the bookkeeping row for one applied migration file.
type schemaMigration struct {
	Version   string `gorm:"primaryKey;size:255"`
	AppliedAt time.Time
}

func (schemaMigration) TableName() string { return "schema_migrations" }

var migrationName = regexp.MustCompile(`^\d{4}_[a-z0-9_]+\.sql$`)

// Migrate applies, in lexical order, every *.sql file in fsys that has not been
// recorded in schema_migrations. Each file runs in its own transaction together
// with its bookkeeping row, so a failure leaves the database at a known
// version. It returns the versions applied by this call.
func Migrate(ctx context.Context, db *gorm.DB, fsys fs.FS) ([]string, error) {
	db = db.WithContext(ctx)

	if err := db.AutoMigrate(&schemaMigration{}); err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}

	var done []schemaMigration
	if err := db.Find(&done).Error; err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	applied := make(map[string]bool, len(done))
	for _, m := range done {
		applied[m.Version] = true
	}

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !migrationName.MatchString(e.Name()) {
			return nil, fmt.Errorf("migration %q does not match NNNN_name.sql", e.Name())
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var ran []string
	for _, name := range names {
		if applied[name] {
			continue
		}
		sqlText, err := fs.ReadFile(fsys, name)
		if err != nil {
			return ran, fmt.Errorf("read %s: %w", name, err)
		}
		err = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(string(sqlText)).Error; err != nil {
				return err
			}
			return tx.Create(&schemaMigration{Version: name, AppliedAt: db.NowFunc()}).Error
		})
		if err != nil {
			return ran, fmt.Errorf("apply %s: %w", name, err)
		}
		ran = append(ran, name)
	}
	return ran, nil
}
