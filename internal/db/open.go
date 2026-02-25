package db

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func Open(cfg Config) (*sql.DB, error) {
	driver := normalizeDriver(cfg.Driver)
	sqlDriver := sqlDriverName(driver)
	if sqlDriver == "" {
		return nil, fmt.Errorf("unsupported DB_DRIVER %q", cfg.Driver)
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
