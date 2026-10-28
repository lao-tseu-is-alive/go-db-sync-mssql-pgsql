package syncer

import (
	"testing"
	"time"
)

type progressTestSource struct {
	rows  [][]any
	index int
}

func (source *progressTestSource) Next() bool { return source.index < len(source.rows) }

func (source *progressTestSource) Values() ([]any, error) {
	values := source.rows[source.index]
	source.index++
	return values, nil
}

func (source *progressTestSource) Err() error   { return nil }
func (source *progressTestSource) Close() error { return nil }

func TestProgressSourceReportsFinalCount(t *testing.T) {
	t.Parallel()
	underlying := &progressTestSource{rows: [][]any{{1}, {2}, {3}}}
	var events []Progress
	source := newProgressSource(underlying, 3, time.Hour, func(event Progress) {
		events = append(events, event)
	})
	for source.Next() {
		if _, err := source.Values(); err != nil {
			t.Fatal(err)
		}
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Rows != 3 || events[0].TotalRows != 3 {
		t.Fatalf("progress events = %#v", events)
	}
}
