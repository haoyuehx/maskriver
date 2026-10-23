package runner

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/pkg/contracts"
	_ "modernc.org/sqlite"
)

// TestOpenForApplyRefusesWALTarget pins the target-side half of the snapshot
// guarantee. A WAL target can accept the masked rows only in its -wal sidecar
// while target.db remains byte-identical; a consumer that copies only the main
// file would still see the original sensitive values even though Apply reported
// Complete. Legacy apply must refuse such a target before opening a Writer.
func TestOpenForApplyRefusesWALTarget(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Runtime{
		Source: filepath.Join(dir, "source.db"), Target: filepath.Join(dir, "target.db"),
		Table: "people", Column: "email",
		Strategy: contracts.StrategyRef{ID: "fake_email", Version: "1"}, KeyID: "synthetic",
		Approved: true, MinSamples: 1, SampleLimit: 10, MinRatio: .9, BatchSize: 2, Workers: 1,
	}
	for _, path := range []string{cfg.Source, cfg.Target} {
		db, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
		if _, err = db.Exec(`INSERT INTO people(id,email) VALUES(1,'synthetic@example.invalid')`); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
		if err = db.Close(); err != nil {
			t.Fatal(err)
		}
	}
	target, err := sql.Open("sqlite", "file:"+cfg.Target+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	var mode string
	if err = target.QueryRow(`PRAGMA journal_mode=WAL`).Scan(&mode); err != nil {
		_ = target.Close()
		t.Fatal(err)
	}
	if mode != "wal" {
		_ = target.Close()
		t.Fatalf("target journal mode = %q, want wal", mode)
	}
	if err = target.Close(); err != nil {
		t.Fatal(err)
	}
	engine, err := OpenForApply(context.Background(), cfg, contracts.Value{})
	if err == nil {
		_ = engine.Close()
		t.Fatal("WAL target was accepted by OpenForApply")
	}
	if !errors.Is(err, contracts.ErrIncomplete) {
		t.Fatalf("WAL target error = %v, want ErrIncomplete", err)
	}
}
