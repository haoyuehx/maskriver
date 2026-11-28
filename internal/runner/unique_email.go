package runner

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// Issue #113 deliberately retains a whole, small table in memory: these are
// transaction-wide collision checks, not streaming or a persistent mapping.
// Both row count and projected bytes are bounded BEFORE values materialise.
const (
	maxUniqueEmailRows     = 1000
	maxUniqueEmailRowBytes = 4096
	maxUniqueEmailDDLBytes = 64 << 10
	maxUniqueEmailIndexes  = 4
	// The frozen WriteTx cannot query its own transaction. Commit rechecks
	// therefore use read-only sessions, which SQLite cache-spill EXCLUSIVE locks
	// could block on large tables. Keep the initial file and index fanout well
	// below the driver's verified default 2 MiB cache, or refuse before writing.
	maxUniqueEmailDatabaseBytes = 512 << 10
)

// SetSQLiteUniqueEmailEnabled explicitly opts this runner into the bounded
// single-table, single-column UNIQUE fake_email path. The default is disabled;
// this does not grant approval or write intent. All existing signed-approval,
// snapshot and target checks still apply. It never enables UNIQUE writes on
// StreamingApply/StreamingResume, and the CLI does not call it. Like the other
// Engine setters, configure it before starting operations, never concurrently.
func (e *Engine) SetSQLiteUniqueEmailEnabled(enabled bool) {
	if e != nil {
		e.uniqueEmailEnabled = enabled
	}
}

func modifiedUnique(table contracts.TablePlan, schema contracts.TableSchema) bool {
	for _, column := range table.Columns {
		for _, unique := range schema.Unique {
			for _, name := range unique {
				if strings.EqualFold(name, column.Column.Column) {
					return true
				}
			}
		}
		for _, index := range schema.Indexes {
			if !index.Unique {
				continue
			}
			for _, name := range index.Columns {
				if strings.EqualFold(name, column.Column.Column) {
					return true
				}
			}
		}
	}
	return false
}

func (e *Engine) checkUniqueEmailScope(ctx context.Context, plan contracts.Plan, schema contracts.TableSchema) error {
	if e.cfg.DialectName() != contracts.SQLite || len(plan.Tables) != 1 || len(plan.Tables[0].Columns) != 1 {
		return contracts.ErrUnsafe
	}
	column := plan.Tables[0].Columns[0]
	if column.Type.Kind != contracts.Text || column.Type.Nullable ||
		column.Strategy != (contracts.StrategyRef{ID: "fake_email", Version: "1"}) {
		return contracts.ErrUnsafe
	}
	for _, unique := range schema.Unique {
		for _, name := range unique {
			if strings.EqualFold(name, column.Column.Column) && len(unique) != 1 {
				return contracts.ErrUnsafe
			}
		}
	}
	// The frozen SQLite DTO drops index collations. An empty Type.Collation is
	// NOT proof of BINARY. Supplement it with read-only catalog queries on BOTH
	// pre-existing paths, without extending contracts or the db module. All row
	// reads and writes still use the existing Reader/Writer/Strategy interfaces.
	if err := e.cfg.Validate(); err != nil {
		return err
	}
	for _, side := range []struct {
		path   string
		reader contracts.Reader
	}{{e.cfg.Source, e.source}, {e.cfg.Target, e.target}} {
		caps, err := sourceCapabilities(side.reader)
		if err != nil {
			return err
		}
		if err := caps.Quiescent(ctx); err != nil {
			return err
		}
		if err := checkUniqueEmailCatalog(ctx, side.path, schema.Ref.Table, column.Column.Column); err != nil {
			return err
		}
	}
	return nil
}

// checkUniqueEmailCatalog opens mode=ro using an escaped absolute file URL. It
// never issues DDL, starts a write transaction, or creates a missing file.
// The supported subset uses default column collation and default ABORT: an
// explicit COLLATE or ON CONFLICT clause anywhere in the table is conservatively
// refused, including explicit BINARY/ABORT. Unique index keys must themselves
// be ordinary ascending BINARY columns, never expressions or partial indexes.
func checkUniqueEmailCatalog(ctx context.Context, path, table, column string) error {
	info, err := os.Stat(path)
	if err != nil {
		return uniqueEmailSQLError(ctx)
	}
	if info.Size() > maxUniqueEmailDatabaseBytes {
		return contracts.ErrUnsafe
	}
	uriPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	u := url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=ro"}
	catalog, err := sql.Open("sqlite", u.String())
	if err != nil {
		return uniqueEmailSQLError(ctx)
	}
	defer catalog.Close()
	catalog.SetMaxOpenConns(1)
	var cacheSize int
	if err := catalog.QueryRowContext(ctx, `PRAGMA main.cache_size`).Scan(&cacheSize); err != nil {
		return uniqueEmailSQLError(ctx)
	}
	// A persistent default_cache_size override can shrink new writer sessions.
	// The existing db factory uses this same driver's default with no cache
	// override; an unrecognised setting is outside this conservative subset.
	if cacheSize != -2000 {
		return contracts.ErrUnsupported
	}
	var ddl sql.NullString
	if err := catalog.QueryRowContext(ctx,
		`SELECT CASE WHEN length(CAST(sql AS BLOB)) <= ? THEN sql END
		 FROM main.sqlite_schema WHERE type='table' AND name=?`, maxUniqueEmailDDLBytes, table).Scan(&ddl); err != nil {
		return uniqueEmailSQLError(ctx)
	}
	if !ddl.Valid || ddl.String == "" || db.CheckMentionsColumn(ddl.String, "collate") || db.CheckMentionsColumn(ddl.String, "conflict") {
		return contracts.ErrUnsupported
	}
	rows, err := catalog.QueryContext(ctx,
		`SELECT name, "unique", partial FROM pragma_index_list(?, 'main') LIMIT ?`, table, maxUniqueEmailIndexes+1)
	if err != nil {
		return uniqueEmailSQLError(ctx)
	}
	var uniqueIndexes []string
	count := 0
	for rows.Next() {
		var name string
		var unique, partial int
		if err := rows.Scan(&name, &unique, &partial); err != nil {
			_ = rows.Close()
			return uniqueEmailSQLError(ctx)
		}
		count++
		if count > maxUniqueEmailIndexes || partial != 0 {
			_ = rows.Close()
			return contracts.ErrUnsupported
		}
		if unique != 0 {
			uniqueIndexes = append(uniqueIndexes, name)
		}
	}
	readErr := rows.Err()
	closeErr := rows.Close()
	if readErr != nil || closeErr != nil {
		return uniqueEmailSQLError(ctx)
	}
	found := false
	for _, index := range uniqueIndexes {
		// Two keys are enough to reject a composite UNIQUE index. A NULL name
		// or negative cid identifies an expression/rowid rather than a column.
		keys, err := catalog.QueryContext(ctx,
			`SELECT cid, name, coll, "desc" FROM pragma_index_xinfo(?, 'main') WHERE key=1 LIMIT 2`, index)
		if err != nil {
			return uniqueEmailSQLError(ctx)
		}
		n := 0
		for keys.Next() {
			var cid, desc int
			var name, coll sql.NullString
			if err := keys.Scan(&cid, &name, &coll, &desc); err != nil {
				_ = keys.Close()
				return uniqueEmailSQLError(ctx)
			}
			n++
			if n != 1 || cid < 0 || !name.Valid || !coll.Valid || !strings.EqualFold(coll.String, "BINARY") || desc != 0 {
				_ = keys.Close()
				return contracts.ErrUnsupported
			}
			if name.String == column {
				found = true
			}
		}
		readErr := keys.Err()
		closeErr := keys.Close()
		if readErr != nil || closeErr != nil {
			return uniqueEmailSQLError(ctx)
		}
		if n != 1 {
			return contracts.ErrUnsupported
		}
	}
	if !found {
		return contracts.ErrUnsupported
	}
	return nil
}

func uniqueEmailSQLError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Driver diagnostics may contain SQL, paths or values. Never forward them.
	return contracts.ErrDatabase
}

// checkUniqueEmailCommit runs while the UPDATEs hold the target's SQLite
// writer lock. Other read-only sessions still see the committed pre-write
// snapshot, but another writer cannot commit new rows/DDL before this batch
// ends. Recheck the source-bound plan and live constraints, then require the
// entire row count to equal the already updated batch. Expected-value UPDATEs
// prove that every original key still existed, so a newly added/removed row
// cannot hide behind an unchanged total count. No successful prefix commits.
// The source remains an externally isolated snapshot, as on the legacy path.
func (e *Engine) checkUniqueEmailCommit(ctx context.Context, plan contracts.Plan, rows int) error {
	unique, err := e.preflightPlan(ctx, plan, true)
	if err != nil {
		return err
	}
	if !unique {
		return contracts.ErrConflict
	}
	for _, side := range []contracts.Reader{e.source, e.target} {
		n, err := side.Count(ctx, plan.Tables[0].Table)
		if err != nil {
			return err
		}
		if n != int64(rows) {
			return contracts.ErrConflict
		}
	}
	return ctx.Err()
}

// uniqueEmailChanges compares every source/target key and original value,
// computes deterministic candidates once, and retains that exact checked batch.
// Its collision proof depends on the catalog's BINARY comparison semantics.
func (e *Engine) uniqueEmailChanges(ctx context.Context, plan contracts.Plan, maxRows int) ([]contracts.RowChange, error) {
	if maxRows < 1 || maxRows > maxUniqueEmailRows {
		return nil, contracts.ErrUnsafe
	}
	table := plan.Tables[0]
	sourceRows, err := e.source.Count(ctx, table.Table)
	if err != nil {
		return nil, err
	}
	targetRows, err := e.target.Count(ctx, table.Table)
	if err != nil {
		return nil, err
	}
	if sourceRows <= 0 || sourceRows != targetRows {
		return nil, contracts.ErrConflict
	}
	if sourceRows > int64(maxRows) {
		return nil, contracts.ErrUnsafe
	}
	changes := make([]contracts.RowChange, 0, int(sourceRows))
	_, err = e.visit(ctx, plan, 0, maxUniqueEmailRowBytes, func(change contracts.RowChange) error {
		if len(changes) >= maxRows {
			return contracts.ErrUnsafe
		}
		changes = append(changes, change)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if int64(len(changes)) != sourceRows {
		return nil, contracts.ErrConflict
	}
	if err := checkUniqueEmailCandidates(ctx, table.Columns[0].Column.Column, changes); err != nil {
		return nil, err
	}
	return changes, nil
}

func checkUniqueEmailCandidates(ctx context.Context, column string, changes []contracts.RowChange) error {
	current := make(map[contracts.Value]bool, len(changes))
	candidates := make(map[contracts.Value]bool, len(changes))
	for _, change := range changes {
		if err := ctx.Err(); err != nil {
			return err
		}
		old, oldOK := change.Expected[column]
		next, nextOK := change.Replacement[column]
		if !oldOK || !nextOK || old.Kind() != contracts.Text || next.Kind() != contracts.Text ||
			old.Payload() == "" || next.Payload() == "" || old == next {
			return contracts.ErrUnsafe
		}
		if current[old] || candidates[next] {
			return contracts.ErrConflict
		}
		current[old], candidates[next] = true, true
	}
	for candidate := range candidates {
		// Refuse even if the occupying row would be changed first: supporting
		// swaps/chains would depend on SQLite's immediate uniqueness checks and
		// row update order. No temporary placeholder or disabled constraint.
		if current[candidate] {
			return contracts.ErrConflict
		}
	}
	return ctx.Err()
}
