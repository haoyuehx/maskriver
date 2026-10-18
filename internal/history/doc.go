// Package history stores explicit reviewer decisions for one immutable plan and
// verifies them before anything else may treat that plan as approved.
//
// # What a record binds
//
// A Record binds every field that says what was reviewed: Plan.ID, the plan
// digests (SchemaDigest, ConfigDigest), SourceSnapshotID, TargetID, the strategy
// ID and version, KeyID, and the exact ordered (table, column) scope list with
// the full three-part table identity. A record that does not match the live plan
// field for field is refused with contracts.ErrConflict, including a reordered
// scope list, a scope list covering only part of a multi-table plan, and any
// digest or identity change. Because the whole ordered scope list is bound,
// approving part of a multi-table plan is not expressible in this version.
//
// The store does not recompute Plan.ID: the canonical plan builder is Main-owned
// (internal/runner). A plan whose ID is not the digest of its own fields is
// therefore only caught by whoever builds and validates that ID; the record still
// binds every component digest, so changing a component invalidates the record.
// Per-column ColumnPlan.Scope and ColumnPlan.NormalizationVersion strings are not
// bound separately; they are covered only insofar as they change ConfigDigest.
//
// # What the signature proves, and what it does not
//
// The trusted Ed25519 public key never travels inside the record: the caller
// supplies it in Config.Trust and the record names it by SignerKeyID. A valid
// signature proves that the holder of that key approved exactly the plan bound in
// this record, issued at the recorded time for the recorded window. It proves
// nothing about the correctness of the classification behind the plan: a
// reviewer, or anyone who obtains the reviewer's key, can approve a wrong plan
// and the signature stays valid. ReviewerID is a self-declared label; it is not
// authenticated beyond "the holder of SignerKeyID signed this".
//
// # What is detectable, and what is not
//
// Detectable: any byte changed in a present record (the detached signature and
// the strict canonical form both break), a record signed by an unknown key or by
// a key the record does not name, a valid signature over a different plan, an
// expired record, a revoked record, and an ambiguous store holding two decodable
// records that claim the same plan.
//
// Not detectable: anything an attacker with write control of the store directory
// does with whole-file operations. This is a local file store, not a
// transparency log. It cannot detect deletion of a record, rollback to an older
// valid record for the same plan, or replacement of the file with any other
// correctly signed record. It cannot detect rollback of a revocation either,
// because the revocation state lives in the very file that is rolled back: a
// revoked record is refused while it is present, but restoring the
// pre-revocation file restores the approval. Revocation must therefore not rely
// on this file alone; a deployment needing anti-rollback guarantees has to
// publish record digests, expiry or revocation to a medium the data owner does
// not control. Nothing here authorises a database write: contracts.Plan is not
// an authorization token, and Apply must still re-check everything it relies on.
//
// # Storage and concurrency
//
// One complete record per plan lives at <Plan.ID>.decision inside the store
// directory: a magic line, the hex Ed25519 signature, and the canonical JSON
// payload. Files are created 0600 inside a directory created 0700. Plan IDs that
// are not a single safe path element are refused, so a hostile plan identifier
// cannot address a file outside the store.
//
// A write stages the whole record in a temporary file in the same directory,
// fsyncs it, renames it over the target and fsyncs the directory. A crash
// therefore leaves either the previous record or the new one, never a partial
// file; an interrupted write leaves no record at all.
//
// There is no lock file. Concurrent writes for one plan are last-writer-wins,
// which is safe here because each file is a complete, self-verifying record, so
// no writer can observe or leave a torn file, and a reader either sees the old
// record or the new one. Two decodable records claiming the same plan under
// different names are refused as ambiguous rather than resolved by file order.
//
// # Writes are explicit
//
// Open performs no I/O and Lookup is read-only, so a read-only flow cannot create
// a history file. Only Approve and Revoke persist anything, and each must be
// called explicitly, so scan, preview, validate and plan building have no path
// into this package's write side. A record stores identifiers and digests only:
// no row values, primary-key values, masking keys, key material or DSNs.
package history
