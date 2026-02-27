package db

import "os"

const (
	defaultSQLiteDSN = "file:arca.db?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
)

type Config struct {
	Driver string
	DSN    string
}

func ConfigFromEnv() Config {
	driver := normalizeDriver(os.Getenv("DB_DRIVER"))

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		switch driver {
		case DriverPostgres:
			dsn = "postgres://localhost:5432/arca?sslmode=disable"
		default:
			dsn = defaultSQLiteDSN
		}
	}

	return Config{
		Driver: driver,
		DSN:    dsn,
	}
}
