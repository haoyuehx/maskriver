package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/haoyuehx/maskriver/internal/testenv"
)

// TestMySQLCLIWorkflow is the M3-2 acceptance path: the real CLI runs
// Scan -> Preview -> Approve -> Apply -> Validate against two distinct,
// disposable MySQL databases created inside the harness-owned container.
// It is opt-in through MASKRIVER_TEST_MYSQL_SOCKET and skips elsewhere.
func TestMySQLCLIWorkflow(t *testing.T) {
	socket := os.Getenv("MASKRIVER_TEST_MYSQL_SOCKET")
	if socket == "" {
		t.Skip("isolated MySQL socket not provided; MySQL CLI UNVERIFIED")
	}
	abs, err := filepath.Abs(socket)
	if err != nil {
		t.Fatal("invalid socket")
	}
	dir := filepath.Dir(abs)
	marker, err := os.ReadFile(filepath.Join(dir, "maskriver-fixture-only"))
	info, statErr := os.Lstat(dir)
	if err != nil || statErr != nil || !info.IsDir() || info.Mode().Perm() != 0700 ||
		string(marker) != "synthetic-only\n" || !strings.HasPrefix(filepath.Base(dir), "mysql-m1a-") {
		t.Fatal("refusing non-harness MySQL instance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cfg := mysql.NewConfig()
	cfg.User = "root"
	cfg.Net = "unix"
	cfg.Addr = abs
	cfg.Timeout = 3 * time.Second
	cfg.ReadTimeout = 10 * time.Second
	cfg.WriteTimeout = 10 * time.Second
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("admin open failed")
	}
	defer admin.Close()
	if err := admin.PingContext(ctx); err != nil {
		t.Fatal("isolated MySQL ping failed")
	}
	suffix := fmt.Sprintf("%d", os.Getpid())
	sourceName := "maskriver_cli_src_" + suffix
	targetName := "maskriver_cli_dst_" + suffix
	for _, name := range []string{sourceName, targetName} {
		if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"); err != nil {
			t.Fatalf("create database failed: %v", err)
		}
	}
	t.Cleanup(func() {
		for _, name := range []string{sourceName, targetName} {
			_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+name+"`")
		}
	})
	for _, name := range []string{sourceName, targetName} {
		conn, err := sql.Open("mysql", dsnFor(cfg, name))
		if err != nil {
			t.Fatal("fixture open failed")
		}
		if _, err := conn.ExecContext(ctx, "SET SESSION time_zone = '+00:00'"); err != nil {
			_ = conn.Close()
			t.Fatal("timezone setup failed")
		}
		if err := testenv.Seed(ctx, conn, "mysql"); err != nil {
			_ = conn.Close()
			t.Fatal(err)
		}
		// The shared fixture uses one birth date for every row. Detection needs
		// distinct evidence, so give each synthetic row its own valid date.
		for i := 1; i <= testenv.Rows; i++ {
			born := time.Date(2000, 2, 29, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i-1).Format(time.DateOnly)
			if _, err := conn.ExecContext(ctx, "UPDATE people SET born=? WHERE id=?", born, i); err != nil {
				_ = conn.Close()
				t.Fatal("synthetic date setup failed")
			}
		}
		// A separate planned table with an outgoing FK proves that source and
		// target FK metadata is compared in logical-schema form.
		if _, err := conn.ExecContext(ctx, "CREATE TABLE notes (id BIGINT PRIMARY KEY, person_id BIGINT NOT NULL, note VARCHAR(128) NOT NULL, FOREIGN KEY(person_id) REFERENCES people(id)) ENGINE=InnoDB"); err != nil {
			_ = conn.Close()
			t.Fatal("notes fixture create failed")
		}
		for i := 1; i <= testenv.Rows; i++ {
			note := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i-1).Format(time.DateOnly)
			if _, err := conn.ExecContext(ctx, "INSERT INTO notes(id,person_id,note) VALUES(?,?,?)", i, i, note); err != nil {
				_ = conn.Close()
				t.Fatal("notes fixture insert failed")
			}
		}
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
	}
	keyFile := filepath.Join(t.TempDir(), "key.bin")
	if err := os.WriteFile(keyFile, []byte(strings.Repeat("M", 40)), 0600); err != nil {
		t.Fatal(err)
	}
	sourceDSN := dsnFor(cfg, sourceName)
	targetDSN := dsnFor(cfg, targetName)
	base := []string{"--dialect", "mysql", "--source", sourceDSN, "--target", targetDSN,
		"--scope", "people.born", "--scope", "notes.note"}
	review := newReviewProvisioning(t)
	runCmd := func(code int, command string, extras ...string) string {
		t.Helper()
		args := append([]string{command}, base...)
		args = append(args, extras...)
		var out, errOut bytes.Buffer
		if got := run(args, &out, &errOut); got != code {
			// The CLI itself is sanitized; do not print args or DSNs.
			t.Fatalf("%s exit=%d wanted=%d; safe output=%q", command, got, code, out.String()+errOut.String())
		}
		all := out.String() + errOut.String()
		for _, secret := range []string{sourceDSN, targetDSN, strings.Repeat("M", 40),
			review.historyDir, review.trustFile, review.reviewerKeyFile} {
			if strings.Contains(all, secret) {
				t.Fatal("CLI leaked a secret or DSN")
			}
		}
		return all
	}
	scanned := runCmd(0, "scan")
	if !strings.Contains(scanned, "complete=true") {
		t.Fatalf("scan did not classify born: %q", scanned)
	}
	common := []string{"--strategy", "cn_date", "--key-id", "synthetic-key", "--key-file", keyFile}
	if got := runCmd(0, "mask", common...); !strings.Contains(got, "dry-run=true") {
		t.Fatal("preview was not a dry run")
	}
	// Preview must not have changed target birth dates.
	var unchanged int
	if err := admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+targetName+"`.people t JOIN `"+sourceName+"`.people s ON t.id=s.id AND t.born=s.born").Scan(&unchanged); err != nil || unchanged != testenv.Rows {
		t.Fatalf("preview changed MySQL target: count=%d err=%v", unchanged, err)
	}
	runCmd(2, "mask", append(append([]string{}, common...), "--apply")...)
	runCmd(1, "mask", append(append(append([]string{}, common...), review.applyFlags()...), "--apply", "--approved")...)
	sourceBeforePeople := tableDigest(t, admin, sourceName, "people", "born")
	sourceBeforeNotes := tableDigest(t, admin, sourceName, "notes", "note")
	review.issueApproval(t, append(append([]string{}, base...), common...))
	if got := runCmd(0, "mask", append(append(append([]string{}, common...), review.applyFlags()...), "--apply", "--approved")...); !strings.Contains(got, "complete=true") {
		t.Fatalf("authorized MySQL apply incomplete: %q", got)
	}
	var sourceDistinct, targetChanged int
	if err := admin.QueryRowContext(ctx, "SELECT COUNT(DISTINCT born) FROM `"+sourceName+"`.people").Scan(&sourceDistinct); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+targetName+"`.people t JOIN `"+sourceName+"`.people s ON t.id=s.id WHERE t.born <> s.born").Scan(&targetChanged); err != nil {
		t.Fatal(err)
	}
	if sourceDistinct != testenv.Rows || targetChanged != testenv.Rows {
		t.Fatalf("MySQL apply touched wrong side: source_distinct=%d target_changed=%d", sourceDistinct, targetChanged)
	}
	if tableDigest(t, admin, sourceName, "people", "born") != sourceBeforePeople ||
		tableDigest(t, admin, sourceName, "notes", "note") != sourceBeforeNotes {
		t.Fatal("MySQL apply changed the source database")
	}
	var notesChanged int
	if err := admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+targetName+"`.notes t JOIN `"+sourceName+"`.notes s ON t.id=s.id WHERE t.note <> s.note").Scan(&notesChanged); err != nil || notesChanged != testenv.Rows {
		t.Fatalf("MySQL notes table was not masked: changed=%d err=%v", notesChanged, err)
	}
	validated := runCmd(0, "validate", append(append([]string{}, common...), "--approved", "--strict")...)
	if !strings.Contains(validated, "pass=true tables=2 rows=50 issues=0") {
		t.Fatalf("strict MySQL validation failed: %q", validated)
	}
	targetPeople := tableDigest(t, admin, targetName, "people", "born")
	targetNotes := tableDigest(t, admin, targetName, "notes", "note")
	// Re-applying the same plan to already-masked data must refuse and write nothing.
	runCmd(1, "mask", append(append(append([]string{}, common...), review.applyFlags()...), "--apply", "--approved")...)
	if tableDigest(t, admin, targetName, "people", "born") != targetPeople ||
		tableDigest(t, admin, targetName, "notes", "note") != targetNotes {
		t.Fatal("refused MySQL re-apply changed the target")
	}
	// Byte-identical DSNs are refused by config before connecting.
	same := append([]string{"scan", "--dialect", "mysql", "--source", sourceDSN, "--target", sourceDSN,
		"--scope", "people.born"}, []string{}...)
	if got := run(same, &bytes.Buffer{}, &bytes.Buffer{}); got != 2 {
		t.Fatalf("same MySQL database accepted: exit=%d", got)
	}
	// An alias of the same physical database is refused after connecting.
	alias := *cfg
	alias.Addr = strings.Replace(abs, "/mysql-", "/./mysql-", 1)
	alias.DBName = sourceName
	aliasDSN := alias.FormatDSN()
	if aliasDSN == sourceDSN || alias.Addr == abs {
		t.Fatal("alias DSN construction failed")
	}
	if got := run([]string{"scan", "--dialect", "mysql", "--source", sourceDSN, "--target", aliasDSN,
		"--scope", "people.born"}, &bytes.Buffer{}, &bytes.Buffer{}); got != 1 {
		t.Fatalf("same MySQL database alias accepted: exit=%d", got)
	}
	// MySQL streaming is refused even with a complete otherwise-valid flag set.
	budgetFile := filepath.Join(t.TempDir(), "budget.json")
	budget := `{"MaxBatchSize":64,"MaxWorkers":1,"MaxRowBytes":1048576,"MaxColumns":16,"MaxTables":4,
		"MaxHistoryEntries":16,"MaxHistoryNameBytes":128,"MaxReceiptPage":8,"MaxIssueCounts":100,
		"MaxQueue":0,"MaxProcessBytes":67108864,"MaxDiskBytes":268435456}`
	if err := os.WriteFile(budgetFile, []byte(budget), 0600); err != nil {
		t.Fatal(err)
	}
	streamArgs := []string{"mask", "--dialect", "mysql", "--source", sourceDSN, "--target", targetDSN,
		"--scope", "people.born", "--strategy", "cn_date", "--key-id", "synthetic-key", "--key-file", keyFile,
		"--streaming", "--apply", "--approved", "--budget-file", budgetFile,
		"--receipt-page-size", "8", "--history-dir", review.historyDir, "--trust-file", review.trustFile}
	if got := run(streamArgs, &bytes.Buffer{}, &bytes.Buffer{}); got != 2 {
		t.Fatalf("MySQL streaming accepted: exit=%d", got)
	}
}

func tableDigest(t *testing.T, db *sql.DB, database, table, column string) string {
	t.Helper()
	var digest string
	query := "SELECT COALESCE(MD5(GROUP_CONCAT(CONCAT_WS('|',id,`" + column + "`) ORDER BY id)), '') FROM `" + database + "`.`" + table + "`"
	if err := db.QueryRow(query).Scan(&digest); err != nil {
		t.Fatalf("fixture digest failed: %v", err)
	}
	return digest
}

func dsnFor(cfg *mysql.Config, database string) string {
	copy := *cfg
	copy.DBName = database
	copy.Timeout = 3 * time.Second
	copy.ReadTimeout = 10 * time.Second
	copy.WriteTimeout = 10 * time.Second
	return copy.FormatDSN()
}
