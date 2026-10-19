package db

import "database/sql"

// DBStats returns the database/sql pool statistics of one Reader or Writer
// opened by this package. The boolean is false for any other implementation,
// including a nil interface or a typed nil, so a caller can report UNVERIFIED
// instead of zero-valued statistics.
//
// This is observation support only: it issues no SQL, takes no lock of its own
// and changes no connection, transaction or pooling behaviour. It exists so
// benchmarks can record the pool state of the real connections the runner uses
// rather than the state of a second, unrelated handle.
func DBStats(source any) (sql.DBStats, bool) {
	switch value := source.(type) {
	case *reader:
		if value == nil || value.adapter == nil {
			return sql.DBStats{}, false
		}
		return value.db.Stats(), true
	case *writer:
		if value == nil || value.adapter == nil {
			return sql.DBStats{}, false
		}
		return value.db.Stats(), true
	case *adapter:
		if value == nil {
			return sql.DBStats{}, false
		}
		return value.db.Stats(), true
	default:
		return sql.DBStats{}, false
	}
}
