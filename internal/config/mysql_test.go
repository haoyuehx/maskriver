package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func mysqlRuntime(source, target string) Runtime {
	return Runtime{
		Dialect: contracts.MySQL, Source: source, Target: target,
		Table: "people", Column: "born",
		MinSamples: 1, SampleLimit: 10, MinRatio: .9, BatchSize: 2, Workers: 1,
	}
}

func TestMySQLRuntimeRequiresDistinctDatabases(t *testing.T) {
	base := "maskriver:SECRET@unix(/tmp/test.sock)/"
	source := base + "source_db?timeout=3s"
	target := base + "target_db?timeout=3s"
	if err := mysqlRuntime(source, target).Validate(); err != nil {
		t.Fatalf("distinct MySQL databases rejected: %v", err)
	}
	if err := mysqlRuntime(source, source).Validate(); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("same MySQL database error=%v, want ErrInvalid", err)
	}
	// A different user or option does not make the same server/database pair a
	// distinct target.
	if err := mysqlRuntime(source, "other:SECRET@unix(/tmp/test.sock)/source_db").Validate(); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("same database under another user error=%v, want ErrInvalid", err)
	}
}

func TestMySQLRuntimeRefusesUnsafeDSN(t *testing.T) {
	for name, dsn := range map[string]string{
		"multi statements": "root@tcp(127.0.0.1:3306)/source_db?multiStatements=true",
		"local infile":     "root@tcp(127.0.0.1:3306)/source_db?allowAllFiles=true",
		"cleartext":        "root@tcp(127.0.0.1:3306)/source_db?allowCleartextPasswords=true",
		"fk off":           "root@tcp(127.0.0.1:3306)/source_db?foreign_key_checks=0",
		"read-only off":    "root@tcp(127.0.0.1:3306)/source_db?tx_read_only=0",
		"sql mode":         "root@tcp(127.0.0.1:3306)/source_db?sql_mode=",
		"set injection":    "root@tcp(127.0.0.1:3306)/source_db?time_zone=UTC,foreign_key_checks=0",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := mysqlRuntime(dsn, "root@tcp(127.0.0.1:3306)/target_db")
			if err := cfg.Validate(); !errors.Is(err, contracts.ErrUnsafe) {
				t.Fatalf("Validate error=%v, want ErrUnsafe", err)
			}
		})
	}
}

func TestMySQLDatabaseOptionsUseLogicalSchema(t *testing.T) {
	source := "root@unix(/tmp/test.sock)/source_db"
	target := "root@unix(/tmp/test.sock)/target_db"
	cfg := mysqlRuntime(source, target)
	opts, err := cfg.DatabaseOptions(true)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Dialect != contracts.MySQL || len(opts.Schemas) != 1 || opts.Schemas[0] != "main" ||
		opts.DSN.Payload() != source {
		t.Fatalf("unexpected MySQL options: %+v", opts)
	}
	// TargetIdentity is stable across password rotation, so an approval is not
	// invalidated merely because a credential changed.
	rotated := mysqlRuntime(source, "root:NEWPASSWORD@unix(/tmp/test.sock)/target_db")
	cfgID, err := cfg.TargetIdentity()
	if err != nil {
		t.Fatal(err)
	}
	rotatedID, err := rotated.TargetIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if cfgID != rotatedID {
		t.Fatal("target identity depended on the password")
	}
	if cfgID == cfg.Source || strings.Contains(cfgID, "NEWPASSWORD") {
		t.Fatal("target identity exposed the raw DSN")
	}
}
