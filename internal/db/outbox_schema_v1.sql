-- MaskRiver streaming receipt outbox, schema version maskriver-outbox-v1.
--
-- This file is executed only by the explicit InstallOutboxSchema migration
-- call. No apply path may execute DDL. Table names are reserved: business
-- scopes must reject them, and the runner must never mask or rewrite them.
--
-- The outbox stores only identifiers, counts, outcomes and timestamps. It must
-- never contain source values, replacement values, primary-key values, masking
-- keys, DSNs or approval private keys.

CREATE TABLE main.maskriver_outbox_schema (
    version    TEXT PRIMARY KEY NOT NULL CHECK (length(version) BETWEEN 1 AND 64),
    created_at TEXT NOT NULL CHECK (length(created_at) BETWEEN 1 AND 40)
) WITHOUT ROWID;

CREATE TABLE main.maskriver_outbox_run (
    run_id            TEXT PRIMARY KEY NOT NULL
                      CHECK (length(run_id) = 32 AND run_id NOT GLOB '*[^0-9a-f]*'),
    protocol          TEXT NOT NULL CHECK (length(protocol) BETWEEN 1 AND 64),
    plan_id           TEXT NOT NULL CHECK (length(plan_id) BETWEEN 1 AND 128),
    approval_binding  TEXT NOT NULL CHECK (length(approval_binding) BETWEEN 1 AND 64),
    target_digest     TEXT NOT NULL CHECK (length(target_digest) BETWEEN 1 AND 128),
    scope_digest      TEXT NOT NULL CHECK (length(scope_digest) BETWEEN 1 AND 128),
    created_at        TEXT NOT NULL CHECK (length(created_at) BETWEEN 1 AND 40)
) WITHOUT ROWID;

CREATE TABLE main.maskriver_outbox_batch (
    run_id          TEXT NOT NULL,
    batch_sequence  INTEGER NOT NULL CHECK (batch_sequence >= 0),
    batch_id        TEXT NOT NULL CHECK (length(batch_id) BETWEEN 1 AND 128),
    table_schema    TEXT NOT NULL CHECK (length(table_schema) BETWEEN 1 AND 128),
    table_name      TEXT NOT NULL CHECK (length(table_name) BETWEEN 1 AND 256),
    affected_rows   INTEGER NOT NULL CHECK (affected_rows >= 0),
    -- Only a receipt whose INSERT is inside the same transaction as the
    -- business UPDATE may be persisted. 1 is contracts.BatchCommitted.
    outcome         INTEGER NOT NULL CHECK (outcome = 1),
    receipt_version TEXT NOT NULL CHECK (receipt_version = 'm2-stream-v1'),
    created_at      TEXT NOT NULL CHECK (length(created_at) BETWEEN 1 AND 40),
    PRIMARY KEY (run_id, batch_sequence),
    UNIQUE (run_id, batch_id),
    FOREIGN KEY (run_id) REFERENCES maskriver_outbox_run(run_id) ON DELETE RESTRICT
) WITHOUT ROWID;
