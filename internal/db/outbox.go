package db

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"modernc.org/sqlite"

	"github.com/haoyuehx/maskriver/internal/testhooks"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// OutboxSchemaVersion is the only streaming outbox schema this package
// understands. A database whose marker row differs is refused before any write.
const OutboxSchemaVersion = "maskriver-outbox-v1"

// OutboxTable* are reserved audit table names. Business scopes must never
// include them: they are not user data and must not be masked, rewritten or
// validated as part of a masking plan.
const (
	outboxSchemaTable = "maskriver_outbox_schema"
	OutboxRunTable    = "maskriver_outbox_run"
	OutboxBatchTable  = "maskriver_outbox_batch"
)

// OutboxTables returns the reserved table names in a stable order.
func OutboxTables() []string {
	return []string{outboxSchemaTable, OutboxRunTable, OutboxBatchTable}
}

// IsOutboxTable reports whether name is any reserved streaming/recovery audit
// table. It covers both schema families so business scopes can never target
// recovery metadata either. OutboxTables keeps its v1 meaning for the outbox
// migration itself.
func IsOutboxTable(name string) bool {
	for _, reserved := range OutboxTables() {
		if name == reserved {
			return true
		}
	}
	for _, reserved := range RecoveryTables() {
		if name == reserved {
			return true
		}
	}
	return false
}

//go:embed outbox_schema_v1.sql
var outboxSchemaSQL string

// OutboxOptions configures the explicit SQLite streaming outbox. It is
// deliberately separate from DatabaseOptions-based OpenWriter: the legacy
// writer never gains outbox behaviour and never requires these settings.
type OutboxOptions struct {
	contracts.DatabaseOptions
	// Protocol must be contracts.StreamingProtocolRevision. No other protocol
	// is implemented by this package.
	Protocol string
	// JournalMode is the required database journal mode. Only "delete" is
	// supported in this version; WAL, memory and off are refused.
	JournalMode string
	// MaxReceiptPage is the largest receipt page this outbox will serve. It must
	// be positive; there is no unbounded default.
	MaxReceiptPage int
}

// OutboxRun is the non-sensitive run identity stored once before batch writes.
// It carries identifiers and digests only; never values, keys or DSNs.
type OutboxRun struct {
	RunID           string
	PlanID          string
	ApprovalBinding string
	TargetDigest    string
	ScopeDigest     string
	Protocol        string
}

// Outbox is a dedicated SQLite streaming receipt writer. All writes run on one
// pinned *sql.Conn whose PRAGMAs were verified at open time, so the same
// connection and the same database transaction carry the business UPDATEs and
// the receipt INSERT.
//
// It is explicitly optional: opening it never changes OpenWriter, and closing
// the legacy path never creates or requires these tables.
type Outbox struct {
	adapter  *adapter
	writer   *writer
	conn     *sql.Conn
	protocol string
	maxPage  int
	path     string
	info     os.FileInfo
	mu       sync.Mutex
	closed   bool
	active   *outboxTx
	hooks    outboxHooks
}

// outboxHooks is a test-only seam for deterministic failure injection. Nil
// hooks mean normal behaviour and are never set in production code.
type outboxHooks struct {
	beforeReceipt func() error
	commit        func(real func() error) error
}

// OpenOutbox opens an explicit SQLite streaming outbox on an existing, isolated
// target database. It fails closed when the platform, path, journal mode or
// schema version is not exactly supported.
func OpenOutbox(ctx context.Context, opts OutboxOptions) (*Outbox, error) {
	if ctx == nil || opts.Protocol != contracts.StreamingProtocolRevision ||
		opts.JournalMode != "delete" || opts.MaxReceiptPage < 1 {
		return nil, contracts.ErrInvalid
	}
	// The write connection is pinned for the whole outbox session, so paged
	// reads need at least one further pooled connection and can never deadlock
	// behind the writer.
	if opts.MaxOpenConns < 2 {
		return nil, contracts.ErrInvalid
	}
	if opts.Dialect != contracts.SQLite || runtime.GOOS != "linux" {
		return nil, contracts.ErrUnsupported
	}
	path, info, err := outboxPathPreflight(opts.DSN.Payload())
	if err != nil {
		return nil, err
	}
	a, err := open(ctx, opts.DatabaseOptions, false, nil)
	if err != nil {
		return nil, err
	}
	conn, err := a.db.Conn(ctx)
	if err != nil {
		_ = a.Close()
		return nil, dbError(err)
	}
	o := &Outbox{
		adapter:  a,
		writer:   &writer{adapter: a},
		conn:     conn,
		protocol: opts.Protocol,
		maxPage:  opts.MaxReceiptPage,
		path:     path,
		info:     info,
	}
	if err := o.verifySession(ctx); err != nil {
		_ = o.Close()
		return nil, err
	}
	if err := o.verifySchema(ctx); err != nil {
		_ = o.Close()
		return nil, err
	}
	if err := o.verifyFileIdentity(); err != nil {
		_ = o.Close()
		return nil, err
	}
	return o, nil
}

// outboxPathPreflight performs the checks that are possible without an
// OS-handle-pinned open: absolute path, existing regular file, not a symlink,
// single link, and a parent directory that is not world-writable. The
// modernc driver reopens databases by pathname, so this reduces accidental and
// simple attacks but does NOT prove resistance to a concurrent path replacer;
// the supported threat boundary is a trusted local directory (ADR-11).
func outboxPathPreflight(dsn string) (string, os.FileInfo, error) {
	path := dsn
	if strings.HasPrefix(path, "file:") {
		normalized, err := sqliteDSN(path, false)
		if err != nil {
			return "", nil, err
		}
		path = strings.TrimPrefix(normalized, "file:")
		if i := strings.IndexByte(path, '?'); i >= 0 {
			path = path[:i]
		}
	}
	if !filepath.IsAbs(path) {
		return "", nil, contracts.ErrInvalid
	}
	link, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil, contracts.ErrNotFound
		}
		return "", nil, contracts.ErrDatabase
	}
	if link.Mode()&os.ModeSymlink != 0 || !link.Mode().IsRegular() {
		return "", nil, contracts.ErrUnsafe
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", nil, contracts.ErrDatabase
	}
	if !os.SameFile(link, info) {
		return "", nil, contracts.ErrUnsafe
	}
	if nlink, ok := linkCount(info); !ok || nlink != 1 {
		return "", nil, contracts.ErrUnsafe
	}
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil || !parent.IsDir() || parent.Mode().Perm()&0o002 != 0 {
		return "", nil, contracts.ErrUnsafe
	}
	return path, info, nil
}

// verifySession checks the pinned connection's effective SQLite settings and
// requires, but never persists, FULL synchronous. journal_mode is never
// changed: an unsupported mode is refused.
func (o *Outbox) verifySession(ctx context.Context) error {
	var mode string
	if err := o.conn.QueryRowContext(ctx, "PRAGMA main.journal_mode").Scan(&mode); err != nil {
		return dbError(err)
	}
	if strings.ToLower(strings.TrimSpace(mode)) != "delete" {
		return contracts.ErrUnsupported
	}
	var sync int
	if err := o.conn.QueryRowContext(ctx, "PRAGMA main.synchronous").Scan(&sync); err != nil {
		return dbError(err)
	}
	if sync != 2 { // 2 == FULL
		if _, err := o.conn.ExecContext(ctx, "PRAGMA main.synchronous=FULL"); err != nil {
			return contracts.ErrUnsupported
		}
		if err := o.conn.QueryRowContext(ctx, "PRAGMA main.synchronous").Scan(&sync); err != nil {
			return dbError(err)
		}
		if sync != 2 {
			return contracts.ErrUnsupported
		}
	}
	var fk int
	if err := o.conn.QueryRowContext(ctx, "PRAGMA main.foreign_keys").Scan(&fk); err != nil {
		return dbError(err)
	}
	if fk != 1 {
		return contracts.ErrUnsupported
	}
	var queryOnly int
	if err := o.conn.QueryRowContext(ctx, "PRAGMA main.query_only").Scan(&queryOnly); err != nil {
		return dbError(err)
	}
	if queryOnly != 0 {
		return contracts.ErrUnsupported
	}
	return nil
}

// verifySchema requires the exact versioned tables. A missing, renamed or
// differently versioned schema fails closed; it is never created here.
func (o *Outbox) verifySchema(ctx context.Context) error {
	for _, name := range OutboxTables() {
		var found string
		err := o.conn.QueryRowContext(ctx,
			"SELECT name FROM main.sqlite_schema WHERE type='table' AND name=?", name).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			return contracts.ErrUnsupported
		}
		if err != nil {
			return dbError(err)
		}
	}
	rows, err := o.conn.QueryContext(ctx, "SELECT version FROM main."+outboxSchemaTable)
	if err != nil {
		return dbError(err)
	}
	defer rows.Close()
	var versions []string
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return dbError(err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return dbError(err)
	}
	if len(versions) != 1 || versions[0] != OutboxSchemaVersion {
		return contracts.ErrUnsupported
	}
	// The version marker alone cannot prove the marker still describes the
	// tables next to it: a modified or replaced run/batch table would otherwise
	// be accepted. Compare the actual column shape with the schema this package
	// writes and fails closed on any difference.
	for _, table := range outboxShape {
		if err := verifyOutboxTableShape(ctx, o.conn, table); err != nil {
			return err
		}
	}
	return nil
}

// outboxColumn is one expected SQLite column definition.
type outboxColumn struct {
	name    string
	typ     string
	notNull int
	pk      int
}

// outboxShape is the exact schema shape written by outbox_schema_v1.sql.
var outboxShape = []struct {
	table   string
	columns []outboxColumn
}{
	{outboxSchemaTable, []outboxColumn{
		{"version", "TEXT", 1, 1},
		{"created_at", "TEXT", 1, 0},
	}},
	{OutboxRunTable, []outboxColumn{
		{"run_id", "TEXT", 1, 1},
		{"protocol", "TEXT", 1, 0},
		{"plan_id", "TEXT", 1, 0},
		{"approval_binding", "TEXT", 1, 0},
		{"target_digest", "TEXT", 1, 0},
		{"scope_digest", "TEXT", 1, 0},
		{"created_at", "TEXT", 1, 0},
	}},
	{OutboxBatchTable, []outboxColumn{
		{"run_id", "TEXT", 1, 1},
		{"batch_sequence", "INTEGER", 1, 2},
		{"batch_id", "TEXT", 1, 0},
		{"table_schema", "TEXT", 1, 0},
		{"table_name", "TEXT", 1, 0},
		{"affected_rows", "INTEGER", 1, 0},
		{"outcome", "INTEGER", 1, 0},
		{"receipt_version", "TEXT", 1, 0},
		{"created_at", "TEXT", 1, 0},
	}},
}

// rowQueryer is satisfied by both *sql.DB and *sql.Conn.
type rowQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// verifyOutboxTableShape compares PRAGMA table_xinfo with the expected
// definition. Column order, names, declared types, NOT NULL and primary-key
// positions must match exactly.
func verifyOutboxTableShape(ctx context.Context, q rowQueryer, table struct {
	table   string
	columns []outboxColumn
}) error {
	rows, err := q.QueryContext(ctx, "PRAGMA main.table_xinfo("+quoteIdentifier(table.table)+")")
	if err != nil {
		return dbError(err)
	}
	defer rows.Close()
	var got []outboxColumn
	for rows.Next() {
		var (
			cid       int
			name, typ string
			notNull   int
			defaultV  sql.NullString
			pk        int
			hidden    int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultV, &pk, &hidden); err != nil {
			return dbError(err)
		}
		if hidden != 0 {
			// A generated or hidden column is never part of the migration shape.
			// Reject it instead of skipping it, otherwise a table could be altered
			// while still matching the expected visible columns.
			return contracts.ErrUnsupported
		}
		got = append(got, outboxColumn{name: name, typ: strings.ToUpper(typ), notNull: notNull, pk: pk})
	}
	if err := rows.Err(); err != nil {
		return dbError(err)
	}
	if len(got) != len(table.columns) {
		return contracts.ErrUnsupported
	}
	for i := range got {
		if got[i] != table.columns[i] {
			return contracts.ErrUnsupported
		}
	}
	return nil
}

// quoteIdentifier quotes a fixed internal identifier.
func quoteIdentifier(name string) string {
	return "\"" + strings.ReplaceAll(name, "\"", "\"\"") + "\""
}

// verifyFileIdentity rechecks that the path still names the same regular file
// that was preflighted at open. It narrows the replacement window but cannot
// close the stat-to-sqlite-open race; see outboxPathPreflight.
func (o *Outbox) verifyFileIdentity() error {
	path := o.adapterPath()
	if path == "" {
		return contracts.ErrUnsafe
	}
	current, err := os.Stat(path)
	if err != nil || !os.SameFile(current, o.info) || !current.Mode().IsRegular() {
		return contracts.ErrUnsafe
	}
	return nil
}

func (o *Outbox) adapterPath() string { return o.path }

// Capabilities reports the verified streaming outbox capability set. It is
// valid only while the outbox is open.
func (o *Outbox) Capabilities() contracts.OutboxCapabilities {
	return contracts.OutboxCapabilities{
		Preprovisioned:        true,
		SameDatabase:          true,
		Transactional:         true,
		SupportsCommitUnknown: true,
		SchemaVersion:         OutboxSchemaVersion,
		ProtocolVersion:       o.protocol,
		MaxReceiptPage:        o.maxPage,
	}
}

// InitializeRun records the non-sensitive run identity in its own autocommit
// statement. Success here means the run row exists; it is never evidence that
// any business batch committed.
func (o *Outbox) InitializeRun(ctx context.Context, run OutboxRun) error {
	if err := o.check(ctx); err != nil {
		return err
	}
	if run.Protocol != o.protocol || !contracts.ValidRunID(run.RunID) ||
		!boundedLabel(run.PlanID, 128) || !boundedLabel(run.ApprovalBinding, 64) ||
		!boundedLabel(run.TargetDigest, 128) || !boundedLabel(run.ScopeDigest, 128) {
		return contracts.ErrInvalid
	}
	if err := o.verifyFileIdentity(); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return contracts.ErrClosed
	}
	_, err := o.conn.ExecContext(ctx,
		"INSERT INTO main."+OutboxRunTable+
			"(run_id,protocol,plan_id,approval_binding,target_digest,scope_digest,created_at) VALUES(?,?,?,?,?,?,?)",
		run.RunID, run.Protocol, run.PlanID, run.ApprovalBinding, run.TargetDigest, run.ScopeDigest, outboxTimestamp())
	if err != nil {
		return classifyOutboxError(err)
	}
	return nil
}

// BeginBatch starts the single target-database transaction for one batch. The
// returned WriteTx must be committed with Commit, which inserts the receipt in
// the same transaction before the one Commit call.
func (o *Outbox) BeginBatch(ctx context.Context, runID string, plan contracts.TablePlan, receipt contracts.StreamingBatchReceipt) (contracts.WriteTx, error) {
	return o.beginBatch(ctx, runID, plan, receipt, nil)
}

// beginBatch is the shared implementation. afterReceipt, when non-nil, runs
// inside the same transaction after the receipt INSERT and before the single
// Commit; the recovery store uses it to update the checkpoint atomically.
func (o *Outbox) beginBatch(ctx context.Context, runID string, plan contracts.TablePlan, receipt contracts.StreamingBatchReceipt, afterReceipt func(context.Context, *sql.Tx) error) (contracts.WriteTx, error) {
	if err := o.check(ctx); err != nil {
		return nil, err
	}
	if !contracts.ValidRunID(runID) || !boundedLabel(receipt.BatchID, 128) ||
		receipt.Ordinal < 0 || receipt.Rows < 0 || receipt.Outcome != contracts.BatchCommitted ||
		receipt.Table != plan.Table {
		return nil, contracts.ErrInvalid
	}
	if IsOutboxTable(plan.Table.Table) {
		return nil, contracts.ErrUnsafe
	}
	if err := o.verifyFileIdentity(); err != nil {
		return nil, err
	}
	keyTypes, changed, ordered, err := o.writer.prepareWrite(ctx, plan)
	if err != nil {
		return nil, err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil, contracts.ErrClosed
	}
	if o.active != nil {
		return nil, contracts.ErrUnsafe
	}
	var exists int
	if err := o.conn.QueryRowContext(ctx, "SELECT 1 FROM main."+OutboxRunTable+" WHERE run_id=?", runID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, contracts.ErrNotFound
		}
		return nil, dbError(err)
	}
	tx, err := o.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, dbError(err)
	}
	base := &writeTx{owner: o.writer, tx: tx, plan: plan, keyTypes: keyTypes, changed: changed, ordered: ordered}
	result := &outboxTx{outbox: o, runID: runID, tx: base, receipt: receipt, afterReceipt: afterReceipt}
	o.writer.mu.Lock()
	o.writer.active = base
	o.writer.mu.Unlock()
	o.active = result
	return result, nil
}

// Page reads one bounded page of receipts for runID, ordered by batch sequence.
// It never materializes the whole run and rejects invalid cursors or oversized
// pages instead of silently truncating them.
func (o *Outbox) Page(ctx context.Context, runID string, afterOrdinal int64, limit int) (contracts.ReceiptPage, error) {
	var empty contracts.ReceiptPage
	if err := o.check(ctx); err != nil {
		return empty, err
	}
	if !contracts.ValidRunID(runID) || afterOrdinal < 0 || limit < 1 || limit > o.maxPage {
		return empty, contracts.ErrInvalid
	}
	var exists int
	if err := o.adapter.db.QueryRowContext(ctx,
		"SELECT 1 FROM main."+OutboxRunTable+" WHERE run_id=?", runID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return empty, contracts.ErrNotFound
		}
		return empty, dbError(err)
	}
	rows, err := o.adapter.db.QueryContext(ctx,
		"SELECT batch_sequence,batch_id,table_schema,table_name,affected_rows,outcome FROM main."+
			OutboxBatchTable+" WHERE run_id=? AND batch_sequence>=? ORDER BY batch_sequence LIMIT ?",
		runID, afterOrdinal, limit+1)
	if err != nil {
		return empty, dbError(err)
	}
	defer rows.Close()
	page := contracts.ReceiptPage{Receipts: make([]contracts.StreamingBatchReceipt, 0, limit+1)}
	for rows.Next() {
		var (
			sequence, rowCount, outcome int64
			batchID, schema, table      string
		)
		if err := rows.Scan(&sequence, &batchID, &schema, &table, &rowCount, &outcome); err != nil {
			return empty, dbError(err)
		}
		page.Receipts = append(page.Receipts, contracts.StreamingBatchReceipt{
			Table:   contracts.TableRef{Database: o.adapter.dataset, Schema: schema, Table: table},
			BatchID: batchID,
			Ordinal: sequence,
			Rows:    rowCount,
			Outcome: contracts.BatchOutcome(outcome),
		})
	}
	if err := rows.Err(); err != nil {
		return empty, dbError(err)
	}
	if len(page.Receipts) > limit {
		page.Receipts = page.Receipts[:limit]
		page.Done = false
	} else {
		page.Done = true
	}
	if len(page.Receipts) > 0 {
		page.NextOrdinal = page.Receipts[len(page.Receipts)-1].Ordinal + 1
	}
	if err := contracts.ValidateReceiptPage(page, afterOrdinal, limit); err != nil {
		return empty, err
	}
	return page, nil
}

// OutboxReceiptReader is a bounded, cursor-based receipt reader for one run.
// It holds no receipt cache: every Page call reads at most the requested number
// of rows plus one look-ahead row.
type OutboxReceiptReader struct {
	outbox *Outbox
	runID  string
	next   int64
	mu     sync.Mutex
	closed bool
}

var _ contracts.ReceiptReader = (*OutboxReceiptReader)(nil)

// Reader returns a bounded page reader for an existing run.
func (o *Outbox) Reader(runID string) (*OutboxReceiptReader, error) {
	if err := o.check(context.Background()); err != nil {
		return nil, err
	}
	if !contracts.ValidRunID(runID) {
		return nil, contracts.ErrInvalid
	}
	var exists int
	if err := o.adapter.db.QueryRow(
		"SELECT 1 FROM main."+OutboxRunTable+" WHERE run_id=?", runID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, contracts.ErrNotFound
		}
		return nil, dbError(err)
	}
	return &OutboxReceiptReader{outbox: o, runID: runID}, nil
}

// Page returns the next bounded page. limit is clamped by the outbox maximum
// and is never interpreted as "all".
func (r *OutboxReceiptReader) Page(ctx context.Context, limit int) (contracts.ReceiptPage, error) {
	var empty contracts.ReceiptPage
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return empty, contracts.ErrClosed
	}
	if r.outbox == nil {
		return empty, contracts.ErrClosed
	}
	if limit < 1 || limit > r.outbox.maxPage {
		return empty, contracts.ErrInvalid
	}
	page, err := r.outbox.Page(ctx, r.runID, r.next, limit)
	if err != nil {
		return empty, err
	}
	if len(page.Receipts) > 0 {
		r.next = page.NextOrdinal
	}
	return page, nil
}

// Close releases the reader. It does not close the outbox.
func (r *OutboxReceiptReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

// Close rolls back any active batch, releases the pinned connection and closes
// the adapter. It is idempotent.
func (o *Outbox) Close() error {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return nil
	}
	o.closed = true
	active := o.active
	o.active = nil
	o.mu.Unlock()
	if active != nil {
		_ = active.Rollback(context.Background())
	}
	var err error
	if o.conn != nil {
		if cerr := o.conn.Close(); cerr != nil {
			err = contracts.ErrDatabase
		}
	}
	if cerr := o.adapter.Close(); cerr != nil {
		err = contracts.ErrDatabase
	}
	return err
}

func (o *Outbox) check(ctx context.Context) error {
	if ctx == nil {
		return contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	o.mu.Lock()
	closed := o.closed
	o.mu.Unlock()
	if closed {
		return contracts.ErrClosed
	}
	return nil
}

type outboxTx struct {
	outbox       *Outbox
	runID        string
	tx           *writeTx
	receipt      contracts.StreamingBatchReceipt
	afterReceipt func(context.Context, *sql.Tx) error
	mu           sync.Mutex
	ended        bool
	updated      int64
}

var _ contracts.WriteTx = (*outboxTx)(nil)

func (t *outboxTx) Update(ctx context.Context, changes []contracts.RowChange) (int64, error) {
	t.mu.Lock()
	ended := t.ended
	t.mu.Unlock()
	if ended {
		return 0, contracts.ErrClosed
	}
	n, err := t.tx.Update(ctx, changes)
	if err != nil {
		return n, err
	}
	t.mu.Lock()
	t.updated += n
	t.mu.Unlock()
	return n, nil
}

// Commit inserts the receipt in the same transaction and then attempts the one
// and only Commit. A known INSERT failure rolls the batch back and is reported
// as a normal error; an error returned by Commit itself is always reported as
// ErrCommitUnknown because the outcome cannot be proven from the error.
func (t *outboxTx) Commit(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended {
		return contracts.ErrClosed
	}
	if ctx == nil {
		t.abortLocked()
		return contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		t.abortLocked()
		return err
	}
	if t.outbox.hooks.beforeReceipt != nil {
		if err := t.outbox.hooks.beforeReceipt(); err != nil {
			t.abortLocked()
			return contracts.ErrDatabase
		}
	}
	if t.updated != t.receipt.Rows {
		// A receipt must not over- or under-state the rows this transaction
		// actually changed; roll back rather than persist a misleading receipt.
		t.abortLocked()
		return contracts.ErrConflict
	}
	if err := t.insertReceipt(ctx); err != nil {
		t.abortLocked()
		return err
	}
	if err := testhooks.Fire("after_receipt_insert"); err != nil {
		t.abortLocked()
		return err
	}
	if t.afterReceipt != nil {
		if err := t.afterReceipt(ctx, t.tx.tx); err != nil {
			t.abortLocked()
			return err
		}
	}
	if err := testhooks.Fire("after_checkpoint_write"); err != nil {
		t.abortLocked()
		return err
	}
	if err := testhooks.Fire("before_commit"); err != nil {
		t.abortLocked()
		return err
	}
	t.tx.mu.Lock()
	if t.tx.ended {
		t.tx.mu.Unlock()
		t.ended = true
		t.outbox.clearActive(t)
		return contracts.ErrClosed
	}
	commit := func() error { return t.tx.tx.Commit() }
	var commitErr error
	if t.outbox.hooks.commit != nil {
		commitErr = t.outbox.hooks.commit(commit)
	} else {
		commitErr = commit()
	}
	t.tx.endLocked()
	t.tx.mu.Unlock()
	t.ended = true
	t.outbox.clearActive(t)
	if commitErr != nil {
		return contracts.ErrCommitUnknown
	}
	_ = testhooks.Fire("after_business_commit")
	// A post-commit hook models "the database committed but the application
	// never received the response". Its error is deliberately ignored: the
	// commit already happened and cannot be reclassified as a failure here.
	_ = testhooks.Fire("after_commit_before_response")
	return nil
}

func (t *outboxTx) Rollback(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended {
		return nil
	}
	if ctx == nil {
		return contracts.ErrInvalid
	}
	t.tx.mu.Lock()
	err := t.tx.tx.Rollback()
	if !t.tx.ended {
		t.tx.endLocked()
	}
	t.tx.mu.Unlock()
	t.ended = true
	t.outbox.clearActive(t)
	if err != nil && !errors.Is(err, sql.ErrTxDone) {
		return contracts.ErrDatabase
	}
	return nil
}

func (t *outboxTx) insertReceipt(ctx context.Context) error {
	receipt := t.receipt
	if !contracts.ValidRunID(t.runID) || !boundedLabel(receipt.BatchID, 128) ||
		receipt.Ordinal < 0 || receipt.Rows < 0 ||
		receipt.Outcome != contracts.BatchCommitted || receipt.Table != t.tx.plan.Table {
		return contracts.ErrInvalid
	}
	_, err := t.tx.tx.ExecContext(ctx,
		"INSERT INTO main."+OutboxBatchTable+
			"(run_id,batch_sequence,batch_id,table_schema,table_name,affected_rows,outcome,receipt_version,created_at) VALUES(?,?,?,?,?,?,?,?,?)",
		t.runID, receipt.Ordinal, receipt.BatchID,
		t.tx.plan.Table.Schema, t.tx.plan.Table.Table, receipt.Rows,
		int64(receipt.Outcome), contracts.StreamingProtocolRevision, outboxTimestamp())
	if err != nil {
		return classifyOutboxError(err)
	}
	return nil
}

func (t *outboxTx) abortLocked() {
	t.tx.mu.Lock()
	_ = t.tx.tx.Rollback()
	if !t.tx.ended {
		t.tx.endLocked()
	}
	t.tx.mu.Unlock()
	t.ended = true
	t.outbox.clearActive(t)
}

func (o *Outbox) clearActive(tx *outboxTx) {
	o.mu.Lock()
	if o.active == tx {
		o.active = nil
	}
	o.mu.Unlock()
	o.writer.mu.Lock()
	o.writer.active = nil
	o.writer.mu.Unlock()
}

// InstallOutboxSchema creates the reserved outbox tables in one explicit
// transaction. It is a migration call, never invoked by an apply path. Existing
// tables are never dropped or altered: an equal version is a no-op, anything
// else is refused.
func InstallOutboxSchema(ctx context.Context, opts OutboxOptions) error {
	if ctx == nil || opts.Dialect != contracts.SQLite {
		return contracts.ErrUnsupported
	}
	a, err := open(ctx, opts.DatabaseOptions, false, nil)
	if err != nil {
		return err
	}
	defer a.Close()
	if _, _, err := outboxPathPreflight(opts.DSN.Payload()); err != nil {
		return err
	}
	var schemaTable string
	err = a.db.QueryRowContext(ctx,
		"SELECT name FROM main.sqlite_schema WHERE type='table' AND name=?", outboxSchemaTable).Scan(&schemaTable)
	if err == nil {
		var existing string
		if err := a.db.QueryRowContext(ctx,
			"SELECT version FROM main."+outboxSchemaTable).Scan(&existing); err != nil {
			return contracts.ErrConflict
		}
		if existing == OutboxSchemaVersion {
			for _, table := range outboxShape {
				if err := verifyOutboxTableShape(ctx, a.db, table); err != nil {
					return err
				}
			}
			return nil
		}
		return contracts.ErrUnsupported
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return dbError(err)
	}
	for _, name := range OutboxTables() {
		var found string
		if err := a.db.QueryRowContext(ctx,
			"SELECT name FROM main.sqlite_schema WHERE type='table' AND name=?", name).Scan(&found); err == nil {
			return contracts.ErrConflict
		}
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return dbError(err)
	}
	defer tx.Rollback()
	for _, statement := range outboxDDL() {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return dbError(err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO main."+outboxSchemaTable+"(version,created_at) VALUES(?,?)",
		OutboxSchemaVersion, outboxTimestamp()); err != nil {
		return dbError(err)
	}
	if err := tx.Commit(); err != nil {
		return contracts.ErrDatabase
	}
	return nil
}

// outboxDDL splits the embedded migration into executable statements. The file
// contains only CREATE TABLE statements; the version marker is inserted
// separately with a timestamp parameter.
func outboxDDL() []string {
	var statements []string
	for _, chunk := range strings.Split(outboxSchemaSQL, ";") {
		var body strings.Builder
		for _, line := range strings.Split(chunk, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "--") {
				continue
			}
			body.WriteString(line)
			body.WriteByte('\n')
		}
		trimmed := strings.TrimSpace(body.String())
		if trimmed == "" {
			continue
		}
		statements = append(statements, trimmed)
	}
	return statements
}

func outboxTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func boundedLabel(value string, max int) bool {
	return value != "" && len(value) <= max && !strings.ContainsRune(value, 0)
}

// classifyOutboxError turns a real SQLite error into a sanitized category. A
// constraint failure is a conflict; a full disk is a database failure. It never
// inspects error text to decide whether a transaction committed.
func classifyOutboxError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var serr *sqlite.Error
	if errors.As(err, &serr) {
		switch serr.Code() & 0xff {
		case 19: // SQLITE_CONSTRAINT
			return contracts.ErrConflict
		case 13: // SQLITE_FULL
			return contracts.ErrDatabase
		}
	}
	return dbError(err)
}
