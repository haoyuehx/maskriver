package db

// Regression tests for the write contract of work package B2a: a NULL original
// must stay NULL, Expected/Replacement must carry every planned column, and the
// generated UPDATE assigns only the columns that actually change. Every fixture
// is synthetic and lives in t.TempDir().

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// writeFixture builds one synthetic SQLite snapshot from a statement list and
// returns the writer options pointing at it plus its path.
func writeFixture(t *testing.T, statements ...string) (contracts.DatabaseOptions, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic-write.db")
	database, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	for _, statement := range statements {
		if _, err = database.Exec(statement); err != nil {
			_ = database.Close()
			t.Fatalf("fixture statement failed: %v", err)
		}
	}
	if err = database.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
	opts := contracts.DatabaseOptions{
		Dialect: contracts.SQLite, DSN: testValue(t, contracts.Text, path),
		DatasetID: "fixture", Schemas: []string{"main"}, MaxOpenConns: 1,
		ConnectTimeout: 5 * time.Second,
	}
	return opts, path
}

// writePlan describes the named columns of one fixture table and builds the
// table plan a caller would hand to Begin.
func writePlan(t *testing.T, ctx context.Context, opened contracts.Writer, name string, key []string, columns ...string) contracts.TablePlan {
	t.Helper()
	schema, err := opened.(*writer).describe(ctx, table(name))
	if err != nil {
		t.Fatalf("describe %s: %v", name, err)
	}
	plan := contracts.TablePlan{Table: table(name), PrimaryKey: append([]string(nil), key...)}
	for _, wanted := range columns {
		var found contracts.Column
		for _, candidate := range schema.Columns {
			if candidate.Ref.Column == wanted {
				found = candidate
			}
		}
		if found.Ref.Column == "" {
			t.Fatalf("fixture table %s has no column %s", name, wanted)
		}
		plan.Columns = append(plan.Columns, contracts.ColumnPlan{Column: found.Ref, Type: found.Type})
	}
	return plan
}

// writeUpdates returns the statements the spy observed with UPDATE intent.
func writeUpdates(spy *SQLSpy) []string {
	updates := make([]string, 0)
	for _, statement := range spy.Writes() {
		if statement.Verb == "UPDATE" {
			updates = append(updates, statement.SQL)
		}
	}
	return updates
}

// writeSetClause extracts the assignment list of a generated UPDATE, so a test
// can prove exactly which columns one statement assigns.
func writeSetClause(t *testing.T, statement string) string {
	t.Helper()
	const prefix, suffix = " SET ", " WHERE "
	if strings.Count(statement, prefix) != 1 || strings.Count(statement, suffix) != 1 {
		t.Fatalf("generated UPDATE is not one parameterised statement: %s", statement)
	}
	rest := statement[strings.Index(statement, prefix)+len(prefix):]
	return rest[:strings.Index(rest, suffix)]
}

// TestWriteTxAssignsOnlyChangedColumns is the B2a writer regression: a row with
// one NULL and one non-NULL planned column writes only the non-NULL column in
// one parameterised UPDATE, and an all-NULL row writes nothing at all.
func TestWriteTxAssignsOnlyChangedColumns(t *testing.T) {
	ctx := context.Background()
	opts, path := writeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL, backup_email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid', NULL)`,
		`INSERT INTO people VALUES (2, NULL, 'b@example.invalid')`,
		`INSERT INTO people VALUES (3, NULL, NULL)`,
		`INSERT INTO people VALUES (4, 'c@example.invalid', 'd@example.invalid')`,
	)
	spy := NewSQLSpy()
	opened, err := spy.OpenWriter(ctx, opts)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer opened.Close()
	plan := writePlan(t, ctx, opened, "people", []string{"id"}, "email", "backup_email")
	tx, err := opened.Begin(ctx, plan)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	changes := []contracts.RowChange{
		{
			Key: []contracts.Value{testValue(t, contracts.Int, "1")},
			// Row 1: email is set, backup_email is NULL and stays NULL.
			Expected:    contracts.Row{"email": testValue(t, contracts.Text, "a@example.invalid"), "backup_email": {}},
			Replacement: contracts.Row{"email": testValue(t, contracts.Text, "masked-1@maskriver.local"), "backup_email": {}},
		},
		{
			Key: []contracts.Value{testValue(t, contracts.Int, "3")},
			// Row 3: every planned column is NULL, so nothing is written.
			Expected:    contracts.Row{"email": {}, "backup_email": {}},
			Replacement: contracts.Row{"email": {}, "backup_email": {}},
		},
		{
			Key: []contracts.Value{testValue(t, contracts.Int, "2")},
			// Row 2: email is NULL and stays NULL, backup_email changes.
			Expected:    contracts.Row{"email": {}, "backup_email": testValue(t, contracts.Text, "b@example.invalid")},
			Replacement: contracts.Row{"email": {}, "backup_email": testValue(t, contracts.Text, "masked-2@maskriver.local")},
		},
		{
			Key: []contracts.Value{testValue(t, contracts.Int, "4")},
			// Row 4: both planned columns change and stay in one UPDATE.
			Expected: contracts.Row{
				"email":        testValue(t, contracts.Text, "c@example.invalid"),
				"backup_email": testValue(t, contracts.Text, "d@example.invalid"),
			},
			Replacement: contracts.Row{
				"email":        testValue(t, contracts.Text, "masked-4@maskriver.local"),
				"backup_email": testValue(t, contracts.Text, "masked-4b@maskriver.local"),
			},
		},
	}
	updated, err := tx.Update(ctx, changes)
	if err != nil || updated != 3 {
		t.Fatalf("update rows=%d err=%v, want 3 changed rows and no error", updated, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	updates := writeUpdates(spy)
	if len(updates) != 3 {
		t.Fatalf("UPDATE statements=%d, want one per changed row (3): %+v", len(updates), updates)
	}
	want := []string{`"email"=?`, `"backup_email"=?`, `"email"=?,"backup_email"=?`}
	for i, statement := range updates {
		if got := writeSetClause(t, statement); got != want[i] {
			t.Fatalf("UPDATE %d SET %s, want only %s", i, got, want[i])
		}
	}

	readback, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open readback: %v", err)
	}
	defer readback.Close()
	var email, backup sql.NullString
	if err = readback.QueryRow(`SELECT email, backup_email FROM people WHERE id=1`).Scan(&email, &backup); err != nil {
		t.Fatalf("read row 1: %v", err)
	}
	if !email.Valid || email.String != "masked-1@maskriver.local" {
		t.Fatalf("row 1 email present=%t, want the masked value", email.Valid)
	}
	if backup.Valid {
		t.Fatal("row 1 lost its NULL backup_email")
	}
	if err = readback.QueryRow(`SELECT email, backup_email FROM people WHERE id=2`).Scan(&email, &backup); err != nil {
		t.Fatalf("read row 2: %v", err)
	}
	if email.Valid {
		t.Fatal("row 2 turned a NULL email into a value")
	}
	if !backup.Valid || backup.String != "masked-2@maskriver.local" {
		t.Fatalf("row 2 backup_email present=%t, want the masked value", backup.Valid)
	}
	if err = readback.QueryRow(`SELECT email, backup_email FROM people WHERE id=3`).Scan(&email, &backup); err != nil {
		t.Fatalf("read row 3: %v", err)
	}
	if email.Valid || backup.Valid {
		t.Fatal("all-NULL row gained a value")
	}
}

// TestWriteTxAllNullRowIssuesNoStatement keeps the all-NULL row rule at the
// writer: a change with nothing to assign issues no UPDATE and leaves the
// single-row fixture file byte identical.
func TestWriteTxAllNullRowIssuesNoStatement(t *testing.T) {
	ctx := context.Background()
	opts, path := writeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO people VALUES (1, NULL)`,
	)
	before := spyTreeDigest(t, filepath.Dir(path))
	spy := NewSQLSpy()
	opened, err := spy.OpenWriter(ctx, opts)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer opened.Close()
	plan := writePlan(t, ctx, opened, "people", []string{"id"}, "email")
	tx, err := opened.Begin(ctx, plan)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	updated, err := tx.Update(ctx, []contracts.RowChange{{
		Key:         []contracts.Value{testValue(t, contracts.Int, "1")},
		Expected:    contracts.Row{"email": {}},
		Replacement: contracts.Row{"email": {}},
	}})
	if err != nil || updated != 0 {
		t.Fatalf("all-NULL update rows=%d err=%v, want 0 rows and no error", updated, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if updates := writeUpdates(spy); len(updates) != 0 {
		t.Fatalf("all-NULL row issued UPDATE statements: %+v", updates)
	}
	if after := spyTreeDigest(t, filepath.Dir(path)); !sameDigest(before, after) {
		t.Fatal("all-NULL row changed the fixture file")
	}
}

// TestWriteTxRefusesNonNullReplacementForNullOriginal keeps a NULL original
// NULL: the writer refuses any non-NULL replacement for it.
func TestWriteTxRefusesNonNullReplacementForNullOriginal(t *testing.T) {
	ctx := context.Background()
	opts, _ := writeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL, backup_email TEXT NULL)`,
		`INSERT INTO people VALUES (1, NULL, NULL)`,
	)
	spy := NewSQLSpy()
	opened, err := spy.OpenWriter(ctx, opts)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer opened.Close()
	plan := writePlan(t, ctx, opened, "people", []string{"id"}, "email", "backup_email")
	tx, err := opened.Begin(ctx, plan)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	_, err = tx.Update(ctx, []contracts.RowChange{{
		Key:         []contracts.Value{testValue(t, contracts.Int, "1")},
		Expected:    contracts.Row{"email": {}, "backup_email": {}},
		Replacement: contracts.Row{"email": testValue(t, contracts.Text, "invented@example.invalid"), "backup_email": {}},
	}})
	if !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("NULL original with a non-NULL replacement error=%v, want ErrUnsafe", err)
	}
	if updates := writeUpdates(spy); len(updates) != 0 {
		t.Fatalf("refused change issued UPDATE statements: %+v", updates)
	}
}

// TestWriteTxRefusesIdentityReplacement keeps the existing gate: a non-NULL
// original whose replacement equals it is never written.
func TestWriteTxRefusesIdentityReplacement(t *testing.T) {
	ctx := context.Background()
	opts, _ := writeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid')`,
	)
	spy := NewSQLSpy()
	opened, err := spy.OpenWriter(ctx, opts)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer opened.Close()
	plan := writePlan(t, ctx, opened, "people", []string{"id"}, "email")
	tx, err := opened.Begin(ctx, plan)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	same := testValue(t, contracts.Text, "a@example.invalid")
	_, err = tx.Update(ctx, []contracts.RowChange{{
		Key:         []contracts.Value{testValue(t, contracts.Int, "1")},
		Expected:    contracts.Row{"email": same},
		Replacement: contracts.Row{"email": same},
	}})
	if !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("identity replacement error=%v, want ErrUnsafe", err)
	}
	if updates := writeUpdates(spy); len(updates) != 0 {
		t.Fatalf("refused change issued UPDATE statements: %+v", updates)
	}
}

// TestWriteTxRequiresEveryPlannedColumn refuses an incomplete RowChange instead
// of defaulting the missing column to NULL.
func TestWriteTxRequiresEveryPlannedColumn(t *testing.T) {
	ctx := context.Background()
	opts, _ := writeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL, backup_email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid', 'b@example.invalid')`,
	)
	opened, err := OpenWriter(ctx, opts)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer opened.Close()
	plan := writePlan(t, ctx, opened, "people", []string{"id"}, "email", "backup_email")
	email := testValue(t, contracts.Text, "a@example.invalid")
	replacementEmail := testValue(t, contracts.Text, "masked@maskriver.local")
	backup := testValue(t, contracts.Text, "b@example.invalid")
	replacementBackup := testValue(t, contracts.Text, "masked-backup@maskriver.local")
	key := []contracts.Value{testValue(t, contracts.Int, "1")}
	for _, tt := range []struct {
		name   string
		change contracts.RowChange
	}{
		{"missing expected column", contracts.RowChange{
			Key: key, Expected: contracts.Row{"email": email},
			Replacement: contracts.Row{"email": replacementEmail, "backup_email": replacementBackup},
		}},
		{"missing replacement column", contracts.RowChange{
			Key: key, Expected: contracts.Row{"email": email, "backup_email": backup},
			Replacement: contracts.Row{"email": replacementEmail},
		}},
		{"unexpected extra column", contracts.RowChange{
			Key: key, Expected: contracts.Row{"email": email, "backup_email": backup, "id": key[0]},
			Replacement: contracts.Row{"email": replacementEmail, "backup_email": replacementBackup},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := opened.Begin(ctx, plan)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			if _, err = tx.Update(ctx, []contracts.RowChange{tt.change}); !errors.Is(err, contracts.ErrInvalid) {
				t.Fatalf("incomplete change error=%v, want ErrInvalid", err)
			}
		})
	}
}

// TestUpdateRejectsEmptyBatch pins the frozen Writer contract: every item in a
// batch must affect exactly one row, so an empty batch is a caller bug and must
// not be reported as a successful no-op.
func TestUpdateRejectsEmptyBatch(t *testing.T) {
	ctx := context.Background()
	opts, _ := writeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid')`,
	)
	writer, err := OpenWriter(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	tx, err := writer.Begin(ctx, writePlan(t, ctx, writer, "people", []string{"id"}, "email"))
	if err != nil {
		t.Fatal(err)
	}
	if n, err := tx.Update(ctx, nil); !errors.Is(err, contracts.ErrInvalid) || n != 0 {
		t.Fatalf("empty batch update=%d err=%v, want 0/ErrInvalid", n, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
}
