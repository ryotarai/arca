package db

import (
	"context"
	"database/sql"
)

// noCancelDB wraps a *sql.DB and strips context cancellation from all
// queries. This prevents a cancelled HTTP request context from calling
// sqlite3_interrupt() on the shared SQLite connection, which would
// break all subsequent queries on that connection.
//
// SQLite queries are local I/O and complete quickly, so honouring
// context cancellation provides little benefit while introducing the
// risk of corrupting the single shared connection.
type noCancelDB struct {
	db *sql.DB
}

func (n *noCancelDB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return n.db.ExecContext(context.WithoutCancel(ctx), query, args...)
}

func (n *noCancelDB) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return n.db.PrepareContext(context.WithoutCancel(ctx), query)
}

func (n *noCancelDB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return n.db.QueryContext(context.WithoutCancel(ctx), query, args...)
}

func (n *noCancelDB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return n.db.QueryRowContext(context.WithoutCancel(ctx), query, args...)
}
