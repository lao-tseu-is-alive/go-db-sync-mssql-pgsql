package syncer

import (
	"context"
	"errors"
	"testing"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/postgres"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/schema"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/stream"
)

type fakeSource struct {
	table      schema.Table
	rowsCalled bool
}

func (source *fakeSource) Describe(context.Context, string, string, string, string) (schema.Table, error) {
	return source.table, nil
}

func (source *fakeSource) Count(context.Context, schema.Table) (int64, error) {
	return 0, nil
}

func (source *fakeSource) Rows(context.Context, schema.Table) (stream.Source, error) {
	source.rowsCalled = true
	return nil, errors.New("unexpected rows call")
}

type fakeTarget struct {
	publishCalled bool
	inspectErr    error
}

func (target *fakeTarget) Inspect(context.Context, schema.Table) (postgres.Status, error) {
	return postgres.Status{Compatible: target.inspectErr == nil}, target.inspectErr
}

func (target *fakeTarget) Publish(context.Context, schema.Table, stream.Source, int64) (postgres.Result, error) {
	target.publishCalled = true
	return postgres.Result{}, nil
}

func TestDryRunNeverOpensRowsOrPublishes(t *testing.T) {
	t.Parallel()
	source := &fakeSource{table: schema.Table{}}
	target := &fakeTarget{}
	report, err := New(source, target).Run(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Applied || source.rowsCalled || target.publishCalled {
		t.Fatalf("dry run mutated or opened rows: %#v", report)
	}
}

func TestIncompatibleTargetStopsBeforeSourceCountAndRows(t *testing.T) {
	t.Parallel()
	source := &fakeSource{table: schema.Table{}}
	target := &fakeTarget{inspectErr: errors.New("incompatible")}
	if _, err := New(source, target).Run(context.Background(), Request{Apply: true}); err == nil {
		t.Fatal("Run unexpectedly accepted incompatible target")
	}
	if source.rowsCalled || target.publishCalled {
		t.Fatal("incompatible target reached transfer")
	}
}
