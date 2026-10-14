package db

import (
	"context"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// TestSQLiteConnectionPoolsClose verifies the actual SQLite/sql.DB pooled
// connections close after Reader/Writer use and after a cancelled request.
// It is a finite connection lifecycle check, not a sustained-leak proof.
func TestSQLiteConnectionPoolsClose(t *testing.T) {
	opts := sqliteOptions(t)
	opts.MaxOpenConns = 3
	for cycle := 0; cycle < 20; cycle++ {
		ctx := context.Background()
		opened, err := OpenReader(ctx, opts)
		if err != nil {
			t.Fatal("reader creation:", err)
		}
		reader := opened.(*reader)
		if n, err := opened.Count(ctx, table("people")); err != nil || n != 25 {
			t.Fatal("reader query:", err)
		}
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := opened.Count(canceled, table("people")); err == nil {
			t.Fatal("cancelled query unexpectedly succeeded")
		}
		if err := opened.Close(); err != nil {
			t.Fatal("reader close:", err)
		}
		if n := reader.db.Stats().OpenConnections; n != 0 {
			t.Fatalf("reader SQL connections retained after Close: %d", n)
		}
		writerIface, err := OpenWriter(ctx, opts)
		if err != nil {
			t.Fatal("writer creation:", err)
		}
		writer := writerIface.(*writer)
		if err := writer.db.PingContext(ctx); err != nil {
			t.Fatal("writer session ping:", err)
		}
		if err := writerIface.Close(); err != nil {
			t.Fatal("writer close:", err)
		}
		if n := writer.db.Stats().OpenConnections; n != 0 {
			t.Fatalf("writer SQL connections retained after Close: %d", n)
		}
	}
}

// Type-level guard keeps a compile-time reference to the real Reader surface.
var _ contracts.Reader = (*reader)(nil)
