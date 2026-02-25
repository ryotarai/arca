package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func ApplyMigrations(ctx context.Context, db *sql.DB) error {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := "migrations/" + entry.Name()
		query, err := migrationFiles.ReadFile(path)
		if err != nil {
			return err
		}

		if _, err := db.ExecContext(ctx, string(query)); err != nil {
			return fmt.Errorf("apply %s: %w", path, err)
		}
	}

	return nil
}
