package contracts

// ValidateShape checks only structural invariants. It does not approve a plan,
// validate digests, verify schema/FKs, or grant database write permission.
func (p Plan) ValidateShape() error {
	if p.Revision != APIRevision || p.ID == "" || p.SourceSnapshotID == "" || p.TargetID == "" || p.SchemaDigest == "" || p.ConfigDigest == "" {
		return ErrInvalid
	}
	if !p.ScanComplete || len(p.Unresolved) != 0 {
		return ErrIncomplete
	}
	if len(p.Tables) == 0 {
		return ErrInvalid
	}
	tables := make(map[TableRef]bool)
	for _, table := range p.Tables {
		if !validTable(table.Table) || tables[table.Table] || len(table.Columns) == 0 {
			return ErrInvalid
		}
		tables[table.Table] = true
		if len(table.PrimaryKey) == 0 {
			return ErrUnsafe
		}
		keys := make(map[string]bool)
		for _, key := range table.PrimaryKey {
			if key == "" || keys[key] {
				return ErrInvalid
			}
			keys[key] = true
		}
		cols := make(map[string]bool)
		for _, col := range table.Columns {
			if col.Column.TableRef != table.Table || col.Column.Column == "" || cols[col.Column.Column] {
				return ErrInvalid
			}
			if keys[col.Column.Column] {
				return ErrUnsafe
			}
			cols[col.Column.Column] = true
			if col.Strategy.ID == "" || col.Strategy.Version == "" || col.Scope == "" || col.NormalizationVersion == "" || col.KeyID == "" || col.Type.Kind == Null || col.Type.Kind > Instant {
				return ErrInvalid
			}
		}
	}
	return nil
}

func validTable(t TableRef) bool { return t.Database != "" && t.Schema != "" && t.Table != "" }
