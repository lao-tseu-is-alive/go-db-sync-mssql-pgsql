package syncer

import (
	"context"
	"errors"
	"testing"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/postgres"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/schema"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/stream"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/syncmode"
)

type fakeSource struct {
	table      schema.Table
	described  int
	counted    int
	rowsCalled bool
	rows       stream.Source
	rowsErr    error
}

func (source *fakeSource) Describe(context.Context, string, string, string, string) (schema.Table, error) {
	source.described++
	return source.table, nil
}

func (source *fakeSource) Count(context.Context, schema.Table) (int64, error) {
	source.counted++
	return 0, nil
}

func (source *fakeSource) Rows(context.Context, schema.Table) (stream.Source, error) {
	source.rowsCalled = true
	return source.rows, source.rowsErr
}

type fakeTarget struct {
	publishCalled bool
	publishMode   syncmode.Mode
	inspectErr    error
}

func (target *fakeTarget) Inspect(context.Context, schema.Table) (postgres.Status, error) {
	return postgres.Status{Compatible: target.inspectErr == nil}, target.inspectErr
}

func (target *fakeTarget) Publish(_ context.Context, _ schema.Table, _ stream.Source, _ int64, mode syncmode.Mode) (postgres.Result, error) {
	target.publishCalled = true
	target.publishMode = mode
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

func TestApplyPassesReplaceModeToTarget(t *testing.T) {
	t.Parallel()
	source := &fakeSource{table: schema.Table{}}
	target := &fakeTarget{}
	report, err := New(source, target).Run(context.Background(), Request{Apply: true, SyncMode: syncmode.Replace})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Applied || !source.rowsCalled || !target.publishCalled || target.publishMode != syncmode.Replace {
		t.Fatalf("replace orchestration report=%#v source=%#v target=%#v", report, source, target)
	}
}

func TestApplyPreparedReusesDiscoveryAndCount(t *testing.T) {
	t.Parallel()
	table := schema.Table{SourceSchema: "dbo", SourceName: "Thing", TargetSchema: "public", TargetName: "thing"}
	source := &fakeSource{table: table}
	target := &fakeTarget{}
	service := New(source, target)
	request := Request{
		SourceSchema: "dbo", SourceTable: "Thing", TargetSchema: "public", TargetTable: "thing",
		Apply: true, SyncMode: syncmode.Replace,
	}
	prepared, err := service.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApplyPrepared(context.Background(), request, prepared); err != nil {
		t.Fatal(err)
	}
	if source.described != 1 || source.counted != 1 || !source.rowsCalled {
		t.Fatalf("source calls: described=%d counted=%d rows=%t", source.described, source.counted, source.rowsCalled)
	}
}

func TestApplyPreparedRejectsDifferentMapping(t *testing.T) {
	t.Parallel()
	table := schema.Table{SourceSchema: "dbo", SourceName: "Thing", TargetSchema: "public", TargetName: "thing"}
	service := New(&fakeSource{table: table}, &fakeTarget{})
	_, err := service.ApplyPrepared(context.Background(), Request{
		SourceSchema: "dbo", SourceTable: "Other", TargetSchema: "public", TargetTable: "thing", Apply: true,
	}, Report{Table: table})
	if err == nil {
		t.Fatal("ApplyPrepared unexpectedly accepted a different mapping")
	}
}
