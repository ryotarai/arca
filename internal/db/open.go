package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func Open(cfg Config) (*sql.DB, error) {
	driver := normalizeDriver(cfg.Driver)
	sqlDriver := sqlDriverName(driver)
	if sqlDriver == "" {
		return nil, fmt.Errorf("unsupported DB_DRIVER %q", cfg.Driver)
	}
	if driver == DriverSQLite {
		if err := ensureSQLiteFileMode(cfg.DSN); err != nil {
			return nil, err
		}
	}

	db, err := sql.Open(sqlDriver, cfg.DSN)
	if err != nil {
		return nil, err
	}

	if driver == DriverSQLite {
		db.SetMaxOpenConns(1)
	}

	return db, nil
}

func ensureSQLiteFileMode(dsn string) error {
	path := sqliteFilePathFromDSN(dsn)
	if path == "" {
		return nil
	}

	file, err := os.OpenFile(path, os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("open sqlite database file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close sqlite database file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("set sqlite database file mode: %w", err)
	}
	return nil
}

func sqliteFilePathFromDSN(dsn string) string {
	if dsn == "" || !strings.HasPrefix(dsn, "file:") {
		return ""
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		return sqlitePathFromRawDSN(dsn)
	}
	if parsed.Opaque != "" {
		return sqlitePathFromRawDSN("file:" + parsed.Opaque)
	}
	if parsed.Path == "" || parsed.Path == ":memory:" {
		return ""
	}
	return parsed.Path
}

func sqlitePathFromRawDSN(dsn string) string {
	raw := strings.TrimPrefix(dsn, "file:")
	if raw == "" {
		return ""
	}
	if idx := strings.IndexByte(raw, '?'); idx >= 0 {
		raw = raw[:idx]
	}
	if raw == "" || raw == ":memory:" {
		return ""
	}
	return raw
}
