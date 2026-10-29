package syncer

import (
	"time"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/stream"
)

const progressClockCheckRows = 8192

type progressSource struct {
	source           stream.Source
	totalRows        int64
	rows             int64
	lastReportedRows int64
	started          time.Time
	lastReportedAt   time.Time
	phase            string
	interval         time.Duration
	callback         func(Progress)
}

func newProgressSource(source stream.Source, totalRows int64, phase string, interval time.Duration, callback func(Progress)) stream.Source {
	now := time.Now()
	return &progressSource{
		source: source, totalRows: totalRows, phase: phase, started: now, lastReportedAt: now,
		interval: interval, callback: callback,
	}
}

func (source *progressSource) Next() bool {
	return source.source.Next()
}

func (source *progressSource) Values() ([]any, error) {
	values, err := source.source.Values()
	if err != nil {
		return nil, err
	}
	source.rows++
	if source.rows%progressClockCheckRows == 0 {
		source.report(false)
	}
	return values, nil
}

func (source *progressSource) Err() error {
	return source.source.Err()
}

func (source *progressSource) Close() error {
	source.report(true)
	return source.source.Close()
}

func (source *progressSource) StreamMetrics() stream.Metrics {
	provider, ok := source.source.(stream.MetricsProvider)
	if !ok {
		return stream.Metrics{}
	}
	return provider.StreamMetrics()
}

func (source *progressSource) report(final bool) {
	now := time.Now()
	if !final && now.Sub(source.lastReportedAt) < source.interval {
		return
	}
	if source.rows == source.lastReportedRows {
		return
	}
	source.callback(Progress{
		Phase: source.phase, Rows: source.rows, TotalRows: source.totalRows,
		Elapsed: now.Sub(source.started),
	})
	source.lastReportedRows = source.rows
	source.lastReportedAt = now
}
