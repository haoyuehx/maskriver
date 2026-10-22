package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/haoyuehx/maskriver/internal/db/mysqlparam"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// Scope is one explicitly declared table/column operation scope.
type Scope struct{ Table, Column string }

// Runtime describes the explicitly scoped table/column operations.
//
// For SQLite, Source and Target are distinct pre-existing disposable database
// files. For MySQL they are DSNs whose database names must identify distinct
// databases; the source and target may live on different servers. Table/Column
// is the single-scope form kept for compatibility; Scopes is the multi-scope
// form. Exactly one of the two forms may be used.
// No configuration option is capable of authorizing a write.
type Runtime struct {
	Dialect        contracts.Dialect
	Source, Target string
	Table, Column  string
	Scopes         []Scope
	Strategy       contracts.StrategyRef
	KeyID          string
	Approved       bool
	MinSamples     int
	SampleLimit    int
	MinRatio       float64
	BatchSize      int
	Workers        int
}

// ResolvedScopes returns the declared scopes in declared order: the Scopes list
// when it is used, otherwise the single Table/Column pair. Declared order is
// the deterministic plan order and is never sorted.
func (c Runtime) ResolvedScopes() []Scope {
	if len(c.Scopes) > 0 {
		return append([]Scope(nil), c.Scopes...)
	}
	return []Scope{{Table: c.Table, Column: c.Column}}
}

// checkedScopes rejects an ambiguous or malformed scope declaration. It is the
// single place the two configuration forms are reconciled.
func (c Runtime) checkedScopes() ([]Scope, error) {
	if len(c.Scopes) > 0 && (c.Table != "" || c.Column != "") {
		return nil, contracts.ErrInvalid
	}
	scopes := c.ResolvedScopes()
	seen := make(map[Scope]bool, len(scopes))
	for _, scope := range scopes {
		if scope.Table == "" || scope.Column == "" || seen[scope] {
			return nil, contracts.ErrInvalid
		}
		seen[scope] = true
	}
	return scopes, nil
}

func (c Runtime) Validate() error {
	scopes, err := c.checkedScopes()
	if err != nil {
		return err
	}
	if len(scopes) == 0 || c.Source == "" || c.Target == "" ||
		c.MinSamples < 1 || c.SampleLimit < c.MinSamples || c.SampleLimit > 10000 ||
		c.MinRatio <= 0 || c.MinRatio > 1 || c.BatchSize < 1 || c.BatchSize > 1000 ||
		c.Workers < 1 || c.Workers > 16 {
		return contracts.ErrInvalid
	}
	switch c.dialect() {
	case contracts.SQLite:
		source, err := os.Stat(c.Source)
		if err != nil || !source.Mode().IsRegular() {
			return contracts.ErrInvalid
		}
		target, err := os.Stat(c.Target)
		if err != nil || !target.Mode().IsRegular() || os.SameFile(source, target) {
			return contracts.ErrInvalid
		}
		if !filepath.IsAbs(c.Source) || !filepath.IsAbs(c.Target) {
			return contracts.ErrInvalid
		}
		return nil
	case contracts.MySQL:
		source, err := mysqlConfig(c.Source)
		if err != nil {
			return err
		}
		target, err := mysqlConfig(c.Target)
		if err != nil {
			return err
		}
		// The same server/database pair can never be a source and target pair,
		// even when the two DSNs name different users or sessions. Different
		// database names on one server are distinct snapshots.
		if source.Net == target.Net && source.Addr == target.Addr &&
			source.DBName == target.DBName {
			return contracts.ErrInvalid
		}
		return nil
	default:
		return contracts.ErrUnsupported
	}
}

func (c Runtime) dialect() contracts.Dialect {
	if c.Dialect == contracts.DialectUnknown {
		return contracts.SQLite
	}
	return c.Dialect
}

// mysqlConfig parses one DSN with the safety options the adapter also enforces.
// It never includes the password in an error or log.
func mysqlConfig(dsn string) (*mysql.Config, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil || cfg.DBName == "" {
		return nil, contracts.ErrInvalid
	}
	if cfg.MultiStatements || cfg.AllowAllFiles || cfg.AllowCleartextPasswords {
		return nil, contracts.ErrUnsafe
	}
	for key, value := range cfg.Params {
		if mysqlparam.Unsafe(key, value) {
			return nil, contracts.ErrUnsafe
		}
	}
	return cfg, nil
}

// SchemaName is the logical schema carried in TableRefs. SQLite uses "main";
// MySQL uses one logical name for both snapshots even when the physical
// database names differ, so the frozen TableRef stays comparable across sides.
func (c Runtime) SchemaName() string { return "main" }

// DialectName reports the effective dialect for runner-specific gates.
func (c Runtime) DialectName() contracts.Dialect { return c.dialect() }

func (c Runtime) DatabaseOptions(source bool) (contracts.DatabaseOptions, error) {
	if err := c.Validate(); err != nil {
		return contracts.DatabaseOptions{}, err
	}
	dsn := c.Target
	if source {
		dsn = c.Source
	}
	value, err := contracts.NewValue(contracts.Text, dsn)
	if err != nil {
		return contracts.DatabaseOptions{}, err
	}
	dialect := c.dialect()
	if dialect != contracts.SQLite && dialect != contracts.MySQL {
		return contracts.DatabaseOptions{}, contracts.ErrUnsupported
	}
	return contracts.DatabaseOptions{
		Dialect: dialect, DSN: value, DatasetID: "maskriver",
		Schemas: []string{c.SchemaName()}, MaxOpenConns: 1, ConnectTimeout: 5 * time.Second,
	}, nil
}

// TargetIdentity is the digest input that names the target without exposing its
// credentials. SQLite keeps the existing absolute path. MySQL binds only the
// normalized network/address/database name, so rotating a password, user,
// timeout or TLS option does not silently change which database a plan approved.
func (c Runtime) TargetIdentity() (string, error) {
	if c.dialect() != contracts.MySQL {
		return c.Target, nil
	}
	cfg, err := mysqlConfig(c.Target)
	if err != nil {
		return "", err
	}
	// Bind only the physical target, not credentials, timeouts or TLS options.
	return (&mysql.Config{Net: cfg.Net, Addr: cfg.Addr, DBName: cfg.DBName}).FormatDSN(), nil
}
