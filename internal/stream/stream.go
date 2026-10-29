// Package stream defines the closeable, bounded-memory row stream shared by database adapters.
package stream

import (
	"time"

	"github.com/jackc/pgx/v5"
)

// Source produces typed rows for pgx COPY and owns an underlying result set.
type Source interface {
	pgx.CopyFromSource
	Close() error
}

// Metrics contains optional timings measured inside a source row stream.
type Metrics struct {
	// Rows is the number of successfully converted source rows.
	Rows int64
	// NextDuration is time in database/sql Rows.Next, including network wait and driver decoding.
	NextDuration time.Duration
	// ScanDuration is time assigning driver values to the generic row buffer.
	ScanDuration time.Duration
	// ConversionDuration is time mapping source values for PostgreSQL COPY.
	ConversionDuration time.Duration
}

// TotalDuration returns all measured source-stream work.
func (metrics Metrics) TotalDuration() time.Duration {
	return metrics.NextDuration + metrics.ScanDuration + metrics.ConversionDuration
}

// MetricsProvider exposes optional source-stream measurements after COPY.
type MetricsProvider interface {
	StreamMetrics() Metrics
}
