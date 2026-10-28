// Package stream defines the closeable, bounded-memory row stream shared by database adapters.
package stream

import "github.com/jackc/pgx/v5"

// Source produces typed rows for pgx COPY and owns an underlying result set.
type Source interface {
	pgx.CopyFromSource
	Close() error
}
