-- MaskRiver local transactional recovery schema, version maskriver-recovery-v1.
--
-- Additive to maskriver-outbox-v1. It is created only by the explicit
-- InstallRecoverySchema migration; no Apply/Preview/Status path executes DDL.
--
-- The tables contain identifiers, digests, counts, states and timestamps only.
-- They must never contain source values, replacement values, plaintext primary
-- keys, masking keys, seeds, DSNs or approval private keys.

CREATE TABLE main.maskriver_recovery_schema (
    version    TEXT PRIMARY KEY NOT NULL CHECK (length(version) BETWEEN 1 AND 64),
    created_at TEXT NOT NULL CHECK (length(created_at) BETWEEN 1 AND 40)
) WITHOUT ROWID;

CREATE TABLE main.maskriver_recovery_manifest (
    run_id                TEXT PRIMARY KEY NOT NULL
                          CHECK (length(run_id) = 32 AND run_id NOT GLOB '*[^0-9a-f]*'),
    protocol              TEXT NOT NULL CHECK (protocol = 'm2-recovery-v1'),
    streaming_protocol    TEXT NOT NULL CHECK (streaming_protocol = 'm2-stream-v1'),
    approval_binding      TEXT NOT NULL CHECK (length(approval_binding) BETWEEN 1 AND 64),
    plan_id               TEXT NOT NULL CHECK (length(plan_id) BETWEEN 1 AND 128),
    plan_content_digest   TEXT NOT NULL CHECK (length(plan_content_digest) BETWEEN 1 AND 128),
    scope_digest          TEXT NOT NULL CHECK (length(scope_digest) BETWEEN 1 AND 128),
    source_snapshot_digest TEXT NOT NULL CHECK (length(source_snapshot_digest) BETWEEN 1 AND 128),
    target_id             TEXT NOT NULL CHECK (length(target_id) BETWEEN 1 AND 128),
    target_identity       TEXT NOT NULL CHECK (length(target_identity) BETWEEN 1 AND 128),
    schema_digest         TEXT NOT NULL CHECK (length(schema_digest) BETWEEN 1 AND 128),
    config_digest         TEXT NOT NULL CHECK (length(config_digest) BETWEEN 1 AND 128),
    strategy_id           TEXT NOT NULL CHECK (length(strategy_id) BETWEEN 1 AND 64),
    strategy_version      TEXT NOT NULL CHECK (length(strategy_version) BETWEEN 1 AND 32),
    normalization_version TEXT NOT NULL CHECK (length(normalization_version) BETWEEN 1 AND 64),
    key_id                TEXT NOT NULL CHECK (length(key_id) BETWEEN 1 AND 128),
    key_check             TEXT NOT NULL
                          CHECK (length(key_check) = 64 AND key_check NOT GLOB '*[^0-9a-f]*'),
    settings_digest       TEXT NOT NULL CHECK (length(settings_digest) BETWEEN 1 AND 128),
    state                 TEXT NOT NULL CHECK (state IN (
                              'new','running','recoverable','recovery_required',
                              'commit_unknown','conflict','validation_failed',
                              'complete','aborted')),
    business_complete     INTEGER NOT NULL CHECK (business_complete IN (0,1)),
    validation_state      TEXT NOT NULL CHECK (validation_state IN ('none','passed','failed')),
    checkpoint_sequence   INTEGER NOT NULL CHECK (checkpoint_sequence >= 0),
    created_at            TEXT NOT NULL CHECK (length(created_at) BETWEEN 1 AND 40),
    updated_at            TEXT NOT NULL CHECK (length(updated_at) BETWEEN 1 AND 40),
    FOREIGN KEY (run_id) REFERENCES maskriver_outbox_run(run_id) ON DELETE RESTRICT
) WITHOUT ROWID;

CREATE TABLE main.maskriver_recovery_progress (
    run_id             TEXT NOT NULL,
    checkpoint_sequence INTEGER NOT NULL CHECK (checkpoint_sequence >= 1),
    kind               TEXT NOT NULL CHECK (kind IN ('batch','progress','boundary')),
    table_ordinal      INTEGER NOT NULL CHECK (table_ordinal >= 0),
    rows_processed     INTEGER NOT NULL CHECK (rows_processed >= 0),
    rows_committed     INTEGER NOT NULL CHECK (rows_committed >= 0),
    committed_batches  INTEGER NOT NULL CHECK (committed_batches >= 0),
    batch_id           TEXT NOT NULL CHECK (length(batch_id) <= 128),
    affected_rows      INTEGER NOT NULL CHECK (affected_rows >= 0),
    business_complete  INTEGER NOT NULL CHECK (business_complete IN (0,1)),
    created_at         TEXT NOT NULL CHECK (length(created_at) BETWEEN 1 AND 40),
    PRIMARY KEY (run_id, checkpoint_sequence),
    FOREIGN KEY (run_id) REFERENCES maskriver_recovery_manifest(run_id) ON DELETE RESTRICT,
    -- A business batch always changed at least one row and carries a batch id.
    -- Progress and boundary events never pretend to have changed rows.
    CHECK ((kind = 'batch' AND affected_rows > 0 AND length(batch_id) > 0)
        OR (kind <> 'batch' AND affected_rows = 0 AND batch_id = '')),
    -- Only the final table boundary may declare business completion.
    CHECK (business_complete = 0 OR kind = 'boundary')
) WITHOUT ROWID;
