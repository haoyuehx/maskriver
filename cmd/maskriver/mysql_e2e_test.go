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
		"--table", "people", "--column", "born"}
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
	if err := admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+targetName+"`.people WHERE born='2000-02-29'").Scan(&unchanged); err != nil || unchanged != testenv.Rows {
		t.Fatalf("preview changed MySQL target: count=%d err=%v", unchanged, err)
	}
	runCmd(2, "mask", append(append([]string{}, common...), "--apply")...)
	runCmd(1, "mask", append(append(append([]string{}, common...), review.applyFlags()...), "--apply", "--approved")...)
	review.issueApproval(t, append(append([]string{}, base...), common...))
	if got := runCmd(0, "mask", append(append(append([]string{}, common...), review.applyFlags()...), "--apply", "--approved")...); !strings.Contains(got, "complete=true") {
		t.Fatalf("authorized MySQL apply incomplete: %q", got)
	}
	var sourceChanged, targetChanged int
	if err := admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+sourceName+"`.people WHERE born <> '2000-02-29'").Scan(&sourceChanged); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+targetName+"`.people WHERE born <> '2000-02-29'").Scan(&targetChanged); err != nil {
		t.Fatal(err)
	}
	if sourceChanged != 0 || targetChanged != testenv.Rows {
		t.Fatalf("MySQL apply touched wrong side: source_changed=%d target_changed=%d", sourceChanged, targetChanged)
	}
	validated := runCmd(0, "validate", append(append([]string{}, common...), "--approved", "--strict")...)
	if !strings.Contains(validated, "pass=true") {
		t.Fatalf("strict MySQL validation failed: %q", validated)
	}
	// Re-applying the same plan to already-masked data must refuse.
	runCmd(1, "mask", append(append(append([]string{}, common...), review.applyFlags()...), "--apply", "--approved")...)
	// Same database pair is never a source and target pair.
	same := append([]string{"scan", "--dialect", "mysql", "--source", sourceDSN, "--target", sourceDSN,
		"--table", "people", "--column", "born"}, []string{}...)
	if got := run(same, &bytes.Buffer{}, &bytes.Buffer{}); got != 2 {
		t.Fatalf("same MySQL database accepted: exit=%d", got)
	}
	// MySQL streaming/recovery has no approved protocol and must be refused.
	if got := run([]string{"mask", "--dialect", "mysql", "--source", sourceDSN, "--target", targetDSN,
		"--table", "people", "--column", "born", "--streaming", "--apply"}, &bytes.Buffer{}, &bytes.Buffer{}); got != 2 {
		t.Fatalf("MySQL streaming accepted: exit=%d", got)
	}
}

func dsnFor(cfg *mysql.Config, database string) string {
	copy := *cfg
	copy.DBName = database
	copy.Timeout = 3 * time.Second
	copy.ReadTimeout = 10 * time.Second
	copy.WriteTimeout = 10 * time.Second
	return copy.FormatDSN()
}
