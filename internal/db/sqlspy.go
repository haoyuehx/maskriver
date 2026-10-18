package db

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"

	"modernc.org/sqlite"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// SQLSpy is test support, not a production connection path. It is an
// instance-level connection factory: the SQLSpy value owns the wrapper, the
// wrapper is passed down the open path by parameter, and nothing is registered
// or reassigned at package level. Tests can therefore run in parallel and under
// -race without sharing mutable state.
//
// A spy-backed Reader or Writer opens the real driver through a
// driver.Connector, so statements observed here are the statements the driver
// really received, not a simulation.
type SQLSpy struct {
	mu         sync.Mutex
	statements []SpyStatement
}

// SpyStatement is one statement observed through a spy-backed connection.
// Verb is the upper-cased leading keyword and Write reports write or
// transaction intent.
type SpyStatement struct {
	SQL   string
	Verb  string
	Write bool
}

// NewSQLSpy returns a spy with an empty statement log.
func NewSQLSpy() *SQLSpy { return &SQLSpy{} }

// Statements returns a copy of every statement observed so far, in the order
// the connections were prepared or begun.
func (s *SQLSpy) Statements() []SpyStatement {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SpyStatement(nil), s.statements...)
}

// Writes returns a copy of the statements classified as write or transaction
// intent.
func (s *SQLSpy) Writes() []SpyStatement {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SpyStatement, 0)
	for _, statement := range s.statements {
		if statement.Write {
			out = append(out, statement)
		}
	}
	return out
}

// OpenReader opens a Reader whose connections are observed by the spy.
// Production callers use OpenReader, which keeps the sql.Open path.
func (s *SQLSpy) OpenReader(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Reader, error) {
	return openReader(ctx, opts, s.connect)
}

// OpenWriter opens a Writer whose connections are observed by the spy.
// Production callers use OpenWriter, which keeps the sql.Open path.
func (s *SQLSpy) OpenWriter(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer, error) {
	return openWriter(ctx, opts, s.connect)
}

// connect wraps the driver connector for an already validated DSN. SQLite is
// the only dialect with a real database in the test environment, so any other
// dialect reports ErrDatabase instead of silently spying on a different driver.
func (s *SQLSpy) connect(driverName, dsn string) (driver.Connector, error) {
	if driverName != "sqlite" {
		return nil, errors.New("sqlspy: unsupported driver")
	}
	base, err := sqlite.NewConnector(dsn)
	if err != nil {
		return nil, err
	}
	return &spyConnector{base: base, spy: s}, nil
}

func (s *SQLSpy) record(statement string) {
	verb, write := classifyStatement(statement)
	s.mu.Lock()
	s.statements = append(s.statements, SpyStatement{SQL: statement, Verb: verb, Write: write})
	s.mu.Unlock()
}

// classifyStatement reports the leading keyword of a statement and whether it
// carries write or transaction intent. The rule is deliberately conservative:
// only SELECT-like statements and the read-only PRAGMA allowlist count as read
// intent. Every statement in a multi-statement text is classified, and anything
// this test support does not recognise counts as write intent, so a new
// statement shape fails the read-only assertions until a reviewer classifies it.
func classifyStatement(statement string) (string, bool) {
	tokens, ok := scanSQLTokens(statement)
	if !ok || len(tokens) == 0 {
		return "", true
	}
	verb, write := classifyTokens(tokens)
	if verb == "" {
		return "", true
	}
	// A single Prepare call can carry several statements; each one must be read
	// intent for the text to count as read intent.
	for _, segment := range splitStatements(tokens) {
		_, segmentWrite := classifyTokens(segment)
		if segmentWrite {
			return verb, true
		}
	}
	return verb, write
}

// splitStatements splits a token stream on top-level ";" separators.
func splitStatements(tokens []string) [][]string {
	segments := make([][]string, 0, 1)
	current := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token == ";" {
			if len(current) > 0 {
				segments = append(segments, current)
			}
			current = make([]string, 0, 8)
			continue
		}
		current = append(current, token)
	}
	if len(current) > 0 {
		segments = append(segments, current)
	}
	return segments
}

// classifyTokens classifies one statement's tokens.
func classifyTokens(tokens []string) (string, bool) {
	if len(tokens) == 0 {
		return "", false
	}
	verb := strings.ToUpper(tokens[0])
	switch verb {
	case "SELECT", "VALUES", "EXPLAIN":
		return verb, false
	case "PRAGMA":
		return verb, !readOnlyPragma(tokens)
	case "WITH":
		for _, token := range tokens {
			if writesData(strings.ToUpper(token)) {
				return verb, true
			}
		}
		return verb, false
	}
	return verb, true
}

// readOnlyPragma accepts only the PRAGMA forms this project uses for metadata
// reads. SQLite also accepts assignment through `PRAGMA name(value)`, which has
// no "=" token, so anything outside this allowlist is write intent.
func readOnlyPragma(tokens []string) bool {
	// Skip "PRAGMA" and an optional schema qualifier ("main.").
	i := 1
	if len(tokens) >= i+2 && tokens[i+1] == "." {
		i += 2
	}
	if i >= len(tokens) {
		return false
	}
	name := strings.ToLower(tokens[i])
	if name == "journal_mode" {
		// A bare journal_mode read is the only read form: journal_mode(value)
		// changes the mode without an "=" token.
		return i+1 == len(tokens)
	}
	switch name {
	case "table_info", "table_list", "table_xinfo", "index_list", "index_xinfo",
		"foreign_key_list", "database_list", "collation_list", "compile_options",
		"function_list", "pragma_list":
	default:
		return false
	}
	for _, token := range tokens[i+1:] {
		if token == "=" {
			return false
		}
	}
	return true
}

// writesData lists the verbs that can change data, schema or transaction state.
func writesData(verb string) bool {
	switch verb {
	case "INSERT", "UPDATE", "DELETE", "REPLACE", "CREATE", "DROP", "ALTER",
		"TRUNCATE", "VACUUM", "REINDEX", "BEGIN", "COMMIT", "ROLLBACK",
		"ATTACH", "DETACH", "SAVEPOINT", "RELEASE":
		return true
	}
	return false
}

// spyConnector forwards to the real driver connector and wraps every
// connection so its statements can be recorded.
type spyConnector struct {
	base driver.Connector
	spy  *SQLSpy
}

func (c *spyConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &spyConn{base: conn, spy: c.spy}, nil
}

func (c *spyConnector) Driver() driver.Driver { return c.base.Driver() }

var (
	_ driver.Conn               = (*spyConn)(nil)
	_ driver.ConnPrepareContext = (*spyConn)(nil)
	_ driver.ConnBeginTx        = (*spyConn)(nil)
)

// spyConn records SQL text when database/sql prepares a statement and records
// BEGIN/COMMIT/ROLLBACK when a transaction is driven. It intentionally does not
// implement driver.QueryerContext, driver.ExecerContext, driver.StmtExecContext
// or driver.StmtQueryContext: database/sql then always falls back to preparing
// the text, which keeps exactly one record per statement and avoids
// reimplementing driver interfaces the wrapped connection may not support.
type spyConn struct {
	base driver.Conn
	spy  *SQLSpy
}

func (c *spyConn) Prepare(query string) (driver.Stmt, error) {
	c.spy.record(query)
	stmt, err := c.base.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &spyStmt{base: stmt}, nil
}

func (c *spyConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	c.spy.record(query)
	var stmt driver.Stmt
	var err error
	if preparer, ok := c.base.(driver.ConnPrepareContext); ok {
		stmt, err = preparer.PrepareContext(ctx, query)
	} else {
		stmt, err = c.base.Prepare(query)
	}
	if err != nil {
		return nil, err
	}
	return &spyStmt{base: stmt}, nil
}

func (c *spyConn) Close() error { return c.base.Close() }

func (c *spyConn) Begin() (driver.Tx, error) {
	c.spy.record("BEGIN")
	tx, err := c.base.Begin()
	if err != nil {
		return nil, err
	}
	return &spyTx{base: tx, spy: c.spy}, nil
}

func (c *spyConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.spy.record("BEGIN")
	var tx driver.Tx
	var err error
	if beginner, ok := c.base.(driver.ConnBeginTx); ok {
		tx, err = beginner.BeginTx(ctx, opts)
	} else {
		tx, err = c.base.Begin()
	}
	if err != nil {
		return nil, err
	}
	return &spyTx{base: tx, spy: c.spy}, nil
}

var _ driver.Stmt = (*spyStmt)(nil)

// spyStmt forwards every statement call to the wrapped driver statement. The
// SQL text was already recorded when the statement was prepared.
type spyStmt struct {
	base driver.Stmt
}

func (s *spyStmt) Close() error  { return s.base.Close() }
func (s *spyStmt) NumInput() int { return s.base.NumInput() }

func (s *spyStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.base.Exec(args)
}

func (s *spyStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.base.Query(args)
}

var _ driver.Tx = (*spyTx)(nil)

type spyTx struct {
	base driver.Tx
	spy  *SQLSpy
}

func (t *spyTx) Commit() error {
	t.spy.record("COMMIT")
	return t.base.Commit()
}

func (t *spyTx) Rollback() error {
	t.spy.record("ROLLBACK")
	return t.base.Rollback()
}
