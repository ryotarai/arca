package db

const (
	DriverSQLite   = "sqlite"
	DriverPostgres = "postgres"
)

func normalizeDriver(driver string) string {
	switch driver {
	case "", "sqlite3", DriverSQLite:
		return DriverSQLite
	case "postgresql", DriverPostgres:
		return DriverPostgres
	default:
		return driver
	}
}

func sqlDriverName(driver string) string {
	switch driver {
	case DriverSQLite:
		return "sqlite"
	case DriverPostgres:
		return "pgx"
	default:
		return ""
	}
}
