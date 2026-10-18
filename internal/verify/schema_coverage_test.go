package verify

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// ---------------------------------------------------------------------------
// Real SQLite fixtures (synthetic data only, disposable files in t.TempDir)
// ---------------------------------------------------------------------------

func accountsTable() contracts.TableRef {
	return contracts.TableRef{Database: "fixture", Schema: "main", Table: "accounts"}
}

func accountsPlan() contracts.Plan {
	t := accountsTable()
	return contracts.Plan{
		Revision:         contracts.APIRevision,
		ID:               "plan-accounts",
		SourceSnapshotID: "snap-accounts",
		TargetID:         "target-accounts",
		SchemaDigest:     "schema-digest",
		ConfigDigest:     "config-digest",
		ScanComplete:     true,
		Tables: []contracts.TablePlan{{
			Table:      t,
			PrimaryKey: []string{"id"},
			Columns: []contracts.ColumnPlan{{
				Column:               contracts.ColumnRef{TableRef: t, Column: "email"},
				Type:                 contracts.Type{Kind: contracts.Text},
				Strategy:             contracts.StrategyRef{ID: "redact", Version: "v1"},
				Scope:                "email",
				NormalizationVersion: "typed-lexical-v1",
				KeyID:                "test-key",
			}},
		}},
	}
}

// openSQLiteFixture seeds a disposable SQLite file with the given statements
// and returns the read-only adapter under test.
func openSQLiteFixture(t *testing.T, statements ...string) contracts.Reader {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic.db")
	seed, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
	if err != nil {
		t.Fatalf("open seed database: %v", err)
	}
	for _, statement := range statements {
		if _, err = seed.Exec(statement); err != nil {
			_ = seed.Close()
			t.Fatalf("seed statement failed: %v", err)
		}
	}
	if err = seed.Close(); err != nil {
		t.Fatalf("close seed database: %v", err)
	}
	opened, err := db.OpenReader(context.Background(), contracts.DatabaseOptions{
		Dialect: contracts.SQLite, DSN: mustValue(t, contracts.Text, path),
		DatasetID: "fixture", Schemas: []string{"main"}, MaxOpenConns: 1,
		ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	return opened
}

func validateAccounts(t *testing.T, source, target contracts.Reader) contracts.ValidationReport {
	t.Helper()
	report, err := New().Validate(context.Background(), source, target,
		contracts.ValidateRequest{Plan: accountsPlan(), RowLimit: 100})
	if err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
	return report
}

const (
	accountsDDL      = `CREATE TABLE accounts (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`
	accountsIndexDDL = `CREATE UNIQUE INDEX accounts_email_key ON accounts(email)`
	accountsRowsDDL  = `INSERT INTO accounts (id, email) VALUES (1,'synthetic@example.invalid'),(2,'second@example.invalid')`
	// maskedRowsDDL simulates a masked target: every value already differs, so
	// only the structural comparison can object to what the copy lost.
	maskedRowsDDL = `INSERT INTO accounts (id, email) VALUES (1,'masked-one@example.invalid'),(2,'masked-two@example.invalid')`
)

// TestValidateSQLiteLostUniqueIndexIsNotPass reproduces the false PASS: the
// masked target lost a UNIQUE index that the source enforces, while the
// adapter reports full index/unique coverage on both sides.
func TestValidateSQLiteLostUniqueIndexIsNotPass(t *testing.T) {
	source := openSQLiteFixture(t, accountsDDL, accountsIndexDDL, accountsRowsDDL)
	target := openSQLiteFixture(t, accountsDDL, maskedRowsDDL)

	report := validateAccounts(t, source, target)

	if report.Passed(true) || report.Passed(false) {
		t.Fatalf("lost UNIQUE index must not pass: issues=%v coverage=%+v", report.Issues, report.Coverage)
	}
	if !hasFailIssue(report, contracts.SchemaMismatch) {
		t.Fatalf("expected Fail/%s, got %v", contracts.SchemaMismatch, report.Issues)
	}
	if report.Coverage.Complete {
		t.Fatal("schema mismatch must keep coverage incomplete")
	}
}

// TestValidateSQLiteExtraUniqueIndexIsNotPass covers the opposite direction: the
// target carries a uniqueness guarantee the source never had.
func TestValidateSQLiteExtraUniqueIndexIsNotPass(t *testing.T) {
	source := openSQLiteFixture(t, accountsDDL, accountsRowsDDL)
	target := openSQLiteFixture(t, accountsDDL, accountsIndexDDL, maskedRowsDDL)

	report := validateAccounts(t, source, target)

	if report.Passed(true) || report.Passed(false) {
		t.Fatalf("extra UNIQUE index must not pass: issues=%v coverage=%+v", report.Issues, report.Coverage)
	}
	if !hasFailIssue(report, contracts.SchemaMismatch) {
		t.Fatalf("expected Fail/%s, got %v", contracts.SchemaMismatch, report.Issues)
	}
}

// TestValidateSQLiteIdenticalConstraintsPass guards against over-rejection: an
// identical schema, including indexes, uniqueness and foreign keys, must pass
// even though every sensitive value changed.
func TestValidateSQLiteIdenticalConstraintsPass(t *testing.T) {
	const schemaStatements = `` +
		`CREATE TABLE orgs (id INTEGER PRIMARY KEY, label TEXT NOT NULL)` +
		`;CREATE TABLE accounts (id INTEGER PRIMARY KEY, email TEXT NOT NULL, org_id INTEGER NOT NULL, ` +
		`FOREIGN KEY(org_id) REFERENCES orgs(id) ON DELETE CASCADE)` +
		`;CREATE UNIQUE INDEX accounts_email_key ON accounts(email)` +
		`;CREATE INDEX accounts_org_idx ON accounts(org_id)` +
		`;INSERT INTO orgs (id, label) VALUES (1,'synthetic')`
	source := openSQLiteFixture(t, schemaStatements+
		`;INSERT INTO accounts (id, email, org_id) VALUES (1,'synthetic@example.invalid',1),`+
		`(2,'second@example.invalid',1),(3,'third@example.invalid',1)`)
	target := openSQLiteFixture(t, schemaStatements+
		`;INSERT INTO accounts (id, email, org_id) VALUES (1,'masked-one@example.invalid',1),`+
		`(2,'masked-two@example.invalid',1),(3,'masked-three@example.invalid',1)`)

	report := validateAccounts(t, source, target)

	if !report.Passed(true) || !report.Coverage.Complete {
		t.Fatalf("identical constraints must pass: issues=%v coverage=%+v", report.Issues, report.Coverage)
	}
}

func hasFailIssue(report contracts.ValidationReport, code contracts.IssueCode) bool {
	for _, iss := range report.Issues {
		if iss.Status == contracts.Fail && iss.Code == code {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Fake-reader coverage gate
// ---------------------------------------------------------------------------

func TestValidateUnknownConstraintCoverageCannotPass(t *testing.T) {
	cases := []struct {
		name string
		miss func(*contracts.SchemaCoverage)
	}{
		{"indexes", func(c *contracts.SchemaCoverage) { c.Indexes = false }},
		{"unique", func(c *contracts.SchemaCoverage) { c.Unique = false }},
		{"foreign_keys", func(c *contracts.SchemaCoverage) { c.ForeignKeys = false }},
	}
	for _, tc := range cases {
		for _, side := range []string{"source", "target"} {
			t.Run(tc.name+"/"+side, func(t *testing.T) {
				ref := peopleTable()
				srcSchema, tgtSchema := peopleSchema(), peopleSchema()
				if side == "source" {
					tc.miss(&srcSchema.Coverage)
				} else {
					tc.miss(&tgtSchema.Coverage)
				}
				rows := []contracts.Row{
					{"id": mustValue(t, contracts.Int, "1"), "email": textV("masked@example.invalid")},
				}
				source, target := newFakeReader(), newFakeReader()
				source.addTable(ref, srcSchema, rows)
				target.addTable(ref, tgtSchema, rows)

				report, err := New().Validate(context.Background(), source, target,
					contracts.ValidateRequest{Plan: peoplePlan(), RowLimit: 10})
				if err != nil {
					t.Fatalf("unexpected validate error: %v", err)
				}
				if !hasIssue(report, contracts.InsufficientEvidence) ||
					!hasIssueStatus(report, contracts.Warning) {
					t.Fatalf("expected Warning/%s, got %v", contracts.InsufficientEvidence, report.Issues)
				}
				if report.Coverage.Complete {
					t.Fatal("unknown constraint coverage must keep coverage incomplete")
				}
				// contracts.ValidationReport.Passed requires Coverage.Complete, so an
				// incomplete report fails both modes (docs/contracts.md §Report gate).
				if report.Passed(false) || report.Passed(true) {
					t.Fatalf("unknown constraint coverage must not pass: %+v", report)
				}
			})
		}
	}
}

// TestValidateCheckConstraintGapStaysUngated pins the open CHECK decision: the
// SQLite adapter reports Coverage.Checks=false unconditionally, so CHECK
// constraints neither gate coverage nor take part in the schema comparison.
// This test changes only when that org-level decision is settled.
func TestValidateCheckConstraintGapStaysUngated(t *testing.T) {
	ref := peopleTable()
	srcSchema, tgtSchema := peopleSchema(), peopleSchema()
	srcSchema.Coverage.Checks, tgtSchema.Coverage.Checks = false, false
	srcSchema.Checks = []string{"id > 0"}

	src := []contracts.Row{{"id": mustValue(t, contracts.Int, "1"), "email": textV("synthetic@example.invalid")}}
	tgt := []contracts.Row{{"id": mustValue(t, contracts.Int, "1"), "email": textV("masked@example.invalid")}}
	source, target := newFakeReader(), newFakeReader()
	source.addTable(ref, srcSchema, src)
	target.addTable(ref, tgtSchema, tgt)

	report, err := New().Validate(context.Background(), source, target,
		contracts.ValidateRequest{Plan: peoplePlan(), RowLimit: 10})
	if err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
	if !report.Passed(true) || !report.Coverage.Complete {
		t.Fatalf("CHECK coverage must stay ungated: issues=%v coverage=%+v", report.Issues, report.Coverage)
	}
}

// ---------------------------------------------------------------------------
// Fake-reader structural divergence
// ---------------------------------------------------------------------------

func constrainedPeopleSchema() contracts.TableSchema {
	schema := peopleSchema()
	schema.Indexes = []contracts.Index{
		{Name: "people_email_name_idx", Columns: []string{"email", "name"}, Unique: true},
	}
	schema.Unique = [][]string{{"email", "name"}}
	schema.ForeignKeys = []contracts.ForeignKey{{
		Columns:       []string{"email", "name"},
		Target:        contracts.TableRef{Database: "synthetic", Schema: "main", Table: "contacts"},
		TargetColumns: []string{"address", "label"},
		OnUpdate:      "NO ACTION",
		OnDelete:      "CASCADE",
	}}
	return schema
}

func TestValidateIndexAndConstraintDivergenceFails(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(tgt *contracts.TableSchema)
	}{
		{"index_removed", func(tgt *contracts.TableSchema) {
			tgt.Indexes = nil
			tgt.Unique = nil
		}},
		{"index_extra", func(tgt *contracts.TableSchema) {
			tgt.Indexes = append(tgt.Indexes,
				contracts.Index{Name: "people_name_idx", Columns: []string{"name"}})
		}},
		{"index_renamed", func(tgt *contracts.TableSchema) { tgt.Indexes[0].Name = "people_other_idx" }},
		{"index_column_order", func(tgt *contracts.TableSchema) {
			tgt.Indexes[0].Columns = []string{"name", "email"}
		}},
		{"index_uniqueness", func(tgt *contracts.TableSchema) { tgt.Indexes[0].Unique = false }},
		{"unique_group_column_order", func(tgt *contracts.TableSchema) {
			tgt.Unique[0] = []string{"name", "email"}
		}},
		{"unique_group_added", func(tgt *contracts.TableSchema) {
			tgt.Unique = append(tgt.Unique, []string{"name"})
		}},
		{"foreign_key_referenced_table", func(tgt *contracts.TableSchema) {
			tgt.ForeignKeys[0].Target.Table = "people_archive"
		}},
		{"foreign_key_target_column_order", func(tgt *contracts.TableSchema) {
			tgt.ForeignKeys[0].TargetColumns = []string{"label", "address"}
		}},
		{"foreign_key_source_column", func(tgt *contracts.TableSchema) {
			tgt.ForeignKeys[0].Columns = []string{"name", "email"}
		}},
		{"foreign_key_on_delete", func(tgt *contracts.TableSchema) {
			tgt.ForeignKeys[0].OnDelete = "SET NULL"
		}},
		{"foreign_key_on_update", func(tgt *contracts.TableSchema) {
			tgt.ForeignKeys[0].OnUpdate = "RESTRICT"
		}},
		{"foreign_key_removed", func(tgt *contracts.TableSchema) { tgt.ForeignKeys = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref := peopleTable()
			srcSchema, tgtSchema := constrainedPeopleSchema(), constrainedPeopleSchema()
			tc.mutate(&tgtSchema)
			src := []contracts.Row{
				{"id": mustValue(t, contracts.Int, "1"), "email": textV("synthetic@example.invalid")},
			}
			tgt := []contracts.Row{
				{"id": mustValue(t, contracts.Int, "1"), "email": textV("masked@example.invalid")},
			}
			source, target := newFakeReader(), newFakeReader()
			source.addTable(ref, srcSchema, src)
			target.addTable(ref, tgtSchema, tgt)

			report, err := New().Validate(context.Background(), source, target,
				contracts.ValidateRequest{Plan: peoplePlan(), RowLimit: 10})
			if err != nil {
				t.Fatalf("unexpected validate error: %v", err)
			}
			if report.Passed(false) || report.Passed(true) {
				t.Fatalf("divergence must not pass: issues=%v", report.Issues)
			}
			if !hasFailIssue(report, contracts.SchemaMismatch) {
				t.Fatalf("expected Fail/%s, got %v", contracts.SchemaMismatch, report.Issues)
			}
		})
	}
}

// TestValidateConstraintOrderingIsIgnored pins the driver-order tolerance: the
// same constraints listed in another order are not a schema mismatch.
func TestValidateConstraintOrderingIsIgnored(t *testing.T) {
	ref := peopleTable()
	emailNameIndex := contracts.Index{Name: "people_email_name_idx", Columns: []string{"email", "name"}, Unique: true}
	nameIndex := contracts.Index{Name: "people_name_idx", Columns: []string{"name"}}
	contactFK := contracts.ForeignKey{
		Columns:       []string{"email", "name"},
		Target:        contracts.TableRef{Database: "synthetic", Schema: "main", Table: "contacts"},
		TargetColumns: []string{"address", "label"},
		OnUpdate:      "NO ACTION",
		OnDelete:      "CASCADE",
	}
	ownerFK := contracts.ForeignKey{
		Columns:       []string{"name"},
		Target:        contracts.TableRef{Database: "synthetic", Schema: "main", Table: "owners"},
		TargetColumns: []string{"alias"},
		OnUpdate:      "CASCADE",
		OnDelete:      "NO ACTION",
	}

	srcSchema := peopleSchema()
	srcSchema.Indexes = []contracts.Index{emailNameIndex, nameIndex}
	srcSchema.Unique = [][]string{{"email", "name"}, {"name"}}
	srcSchema.ForeignKeys = []contracts.ForeignKey{contactFK, ownerFK}

	tgtSchema := peopleSchema()
	tgtSchema.Indexes = []contracts.Index{nameIndex, emailNameIndex}
	tgtSchema.Unique = [][]string{{"name"}, {"email", "name"}}
	tgtSchema.ForeignKeys = []contracts.ForeignKey{ownerFK, contactFK}

	src := []contracts.Row{{"id": mustValue(t, contracts.Int, "1"), "email": textV("synthetic@example.invalid")}}
	tgt := []contracts.Row{{"id": mustValue(t, contracts.Int, "1"), "email": textV("masked@example.invalid")}}
	source, target := newFakeReader(), newFakeReader()
	source.addTable(ref, srcSchema, src)
	target.addTable(ref, tgtSchema, tgt)

	report, err := New().Validate(context.Background(), source, target,
		contracts.ValidateRequest{Plan: peoplePlan(), RowLimit: 10})
	if err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
	if !report.Passed(true) || !report.Coverage.Complete {
		t.Fatalf("reordered constraints must pass: issues=%v coverage=%+v", report.Issues, report.Coverage)
	}
}

// TestValidateDuplicateUniqueGroupsStillPass is the P1 regression from review:
// a table may legitimately carry two UNIQUE indexes over the same column list
// (different names, identical groups). Both sides must still compare equal.
func TestValidateDuplicateUniqueGroupsStillPass(t *testing.T) {
	ref := peopleTable()
	build := func() contracts.TableSchema {
		schema := peopleSchema()
		schema.Indexes = []contracts.Index{
			{Name: "people_email_a", Columns: []string{"email"}, Unique: true},
			{Name: "people_email_b", Columns: []string{"email"}, Unique: true},
		}
		schema.Unique = [][]string{{"email"}, {"email"}}
		return schema
	}
	srcSchema, tgtSchema := build(), build()
	src := []contracts.Row{{"id": mustValue(t, contracts.Int, "1"), "email": textV("synthetic@example.invalid")}}
	tgt := []contracts.Row{{"id": mustValue(t, contracts.Int, "1"), "email": textV("masked@example.invalid")}}
	source, target := newFakeReader(), newFakeReader()
	source.addTable(ref, srcSchema, src)
	target.addTable(ref, tgtSchema, tgt)

	report, err := New().Validate(context.Background(), source, target,
		contracts.ValidateRequest{Plan: peoplePlan(), RowLimit: 10})
	if err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
	if !report.Passed(true) || !report.Coverage.Complete {
		t.Fatalf("duplicate identical unique groups must pass: issues=%v coverage=%+v", report.Issues, report.Coverage)
	}
}

// TestValidateIndexNameSubstitutionCannotPass guards the asymmetric case: the
// same number of indexes with different names must not compare equal.
func TestValidateIndexNameSubstitutionCannotPass(t *testing.T) {
	ref := peopleTable()
	srcSchema, tgtSchema := peopleSchema(), peopleSchema()
	srcSchema.Indexes = []contracts.Index{{Name: "people_email_idx", Columns: []string{"email"}}}
	tgtSchema.Indexes = []contracts.Index{{Name: "people_other_idx", Columns: []string{"email"}}}
	src := []contracts.Row{{"id": mustValue(t, contracts.Int, "1"), "email": textV("synthetic@example.invalid")}}
	tgt := []contracts.Row{{"id": mustValue(t, contracts.Int, "1"), "email": textV("masked@example.invalid")}}
	source, target := newFakeReader(), newFakeReader()
	source.addTable(ref, srcSchema, src)
	target.addTable(ref, tgtSchema, tgt)

	report, err := New().Validate(context.Background(), source, target,
		contracts.ValidateRequest{Plan: peoplePlan(), RowLimit: 10})
	if err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
	if report.Passed(true) {
		t.Fatalf("renamed index must not pass: issues=%v", report.Issues)
	}
	if !hasFailIssue(report, contracts.SchemaMismatch) {
		t.Fatalf("expected Fail/%s, got %v", contracts.SchemaMismatch, report.Issues)
	}
}

// TestValidateDuplicateIndexNameCannotPass covers a duplicated name on one side
// only; the multiset comparison must object in both directions.
func TestValidateDuplicateIndexNameCannotPass(t *testing.T) {
	ref := peopleTable()
	srcSchema, tgtSchema := peopleSchema(), peopleSchema()
	one := contracts.Index{Name: "people_email_idx", Columns: []string{"email"}}
	srcSchema.Indexes = []contracts.Index{one, one}
	tgtSchema.Indexes = []contracts.Index{one, {Name: "people_email_idx2", Columns: []string{"email"}}}
	src := []contracts.Row{{"id": mustValue(t, contracts.Int, "1"), "email": textV("synthetic@example.invalid")}}
	tgt := []contracts.Row{{"id": mustValue(t, contracts.Int, "1"), "email": textV("masked@example.invalid")}}
	source, target := newFakeReader(), newFakeReader()
	source.addTable(ref, srcSchema, src)
	target.addTable(ref, tgtSchema, tgt)

	report, err := New().Validate(context.Background(), source, target,
		contracts.ValidateRequest{Plan: peoplePlan(), RowLimit: 10})
	if err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
	if report.Passed(true) {
		t.Fatalf("asymmetric duplicate index name must not pass: issues=%v", report.Issues)
	}
}
