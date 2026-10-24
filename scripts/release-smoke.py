#!/usr/bin/env python3
"""Release-binary smoke test for a MaskRiver release package.

Runs the extracted release binary (not `go run`) against synthetic, disposable
SQLite databases. It exercises M1 scan/preview/approve/apply/validate and the
M2 streaming + interruption + status + resume + validate + idempotent-resume
path. The interruption is a real SIGKILL of the release binary after a committed
checkpoint is observed; no test-only fault hook is used.

Requires Python 3 with the `cryptography` package for Ed25519 key generation.
Only synthetic data is used.
"""
import argparse
import hashlib
import json
import os
import shutil
import signal
import sqlite3
import subprocess
import sys
import tempfile
import time

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey


def run(binary, args, expect=0):
    proc = subprocess.run([binary] + args, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    if proc.returncode != expect:
        raise SystemExit(f"FAIL: {' '.join(args)} exit={proc.returncode} want={expect}\n{proc.stdout}")
    return proc.stdout


def run_fail(binary, args, fragment):
    proc = subprocess.run([binary] + args, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    if proc.returncode == 0:
        raise SystemExit(f"FAIL: command unexpectedly succeeded: {' '.join(args)}\n{proc.stdout}")
    if fragment.lower() not in proc.stdout.lower():
        raise SystemExit(
            f"FAIL: refusal did not contain {fragment!r}: {' '.join(args)}\n{proc.stdout}"
        )
    return proc.stdout


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def create_db(path, rows):
    if os.path.exists(path):
        os.unlink(path)
    db = sqlite3.connect(path)
    db.execute("PRAGMA journal_mode=DELETE")
    db.execute("CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL)")
    db.executemany(
        "INSERT INTO people(id,email) VALUES(?,?)",
        [(i, f"smoke{i:05d}@example.invalid") for i in range(1, rows + 1)],
    )
    db.commit()
    db.close()


def emails(path):
    db = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    try:
        return [row[0] for row in db.execute("SELECT email FROM people ORDER BY id")]
    finally:
        db.close()


def recovery_state(target, run_id):
    db = sqlite3.connect(f"file:{target}?mode=ro", uri=True)
    try:
        row = db.execute(
            "SELECT checkpoint_sequence,state,business_complete,validation_state "
            "FROM maskriver_recovery_manifest WHERE run_id=?", (run_id,)
        ).fetchone()
        receipts = db.execute(
            "SELECT COUNT(*),COALESCE(SUM(affected_rows),0) FROM maskriver_outbox_batch WHERE run_id=?",
            (run_id,),
        ).fetchone()
        return row, receipts
    finally:
        db.close()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--package", required=True, help="extracted package directory")
    parser.add_argument("--work", required=True, help="empty work directory")
    parser.add_argument("--rows", type=int, default=2000)
    args = parser.parse_args()

    binary = os.path.join(args.package, "maskriver")
    if not os.access(binary, os.X_OK):
        raise SystemExit("FAIL: package binary is missing or not executable")
    work = args.work
    if os.path.isdir(work) and os.listdir(work):
        raise SystemExit("FAIL: work directory must be empty")
    os.makedirs(work, exist_ok=True)

    source = os.path.join(work, "source.db")
    target_m1 = os.path.join(work, "target-m1.db")
    create_db(source, args.rows)
    shutil.copyfile(source, target_m1)
    source_before = sha256(source)

    key_file = os.path.join(work, "masking.key")
    with open(key_file, "wb") as f:
        f.write(b"K" * 40)

    private = Ed25519PrivateKey.generate()
    seed = private.private_bytes(
        encoding=serialization.Encoding.Raw,
        format=serialization.PrivateFormat.Raw,
        encryption_algorithm=serialization.NoEncryption(),
    )
    reviewer_key = os.path.join(work, "reviewer.key")
    with open(reviewer_key, "wb") as f:
        f.write(seed)
    public = private.public_key().public_bytes(
        encoding=serialization.Encoding.Raw,
        format=serialization.PublicFormat.Raw,
    )
    trust_file = os.path.join(work, "trust.json")
    with open(trust_file, "w", encoding="utf-8") as f:
        json.dump({"version": "maskriver-trust-v1", "keys": [{"signer_key_id": "smoke-reviewer", "public_key_hex": public.hex()}]}, f)
    history = os.path.join(work, "history")
    os.makedirs(history, exist_ok=True)

    common = [
        "--source", source, "--target", target_m1,
        "--min-samples", "1", "--sample-limit", "10000", "--scope", "people.email",
    ]
    plan = common + ["--strategy", "fake_email", "--key-id", "smoke-key"]
    review = [
        "--trust-file", trust_file, "--history-dir", history,
        "--reviewer-key-id", "smoke-reviewer", "--reviewer-key-file", reviewer_key,
        "--reviewer-id", "smoke@example.invalid",
    ]

    print("== M1 scan/preview/approve/apply/validate ==")
    run(binary, ["scan"] + common)
    run(binary, ["mask"] + plan + ["--key-file", key_file])
    run(binary, ["approve"] + plan + review)
    run(binary, ["mask"] + plan + ["--key-file", key_file] + review[:4] + ["--approved", "--apply"])
    run(binary, ["validate"] + plan + ["--key-file", key_file, "--strict"])
    if sha256(source) != source_before:
        raise SystemExit("FAIL: source changed during M1 workflow")
    if emails(source) == emails(target_m1):
        raise SystemExit("FAIL: M1 target was not masked")

    print("== WAL target refusal (v1.0.1 fail-closed regression) ==")
    target_wal = os.path.join(work, "target-wal.db")
    shutil.copyfile(source, target_wal)
    wal_db = sqlite3.connect(target_wal)
    try:
        mode = wal_db.execute("PRAGMA journal_mode=WAL").fetchone()[0]
    finally:
        wal_db.close()
    if str(mode).lower() != "wal":
        raise SystemExit(f"FAIL: could not switch target to WAL: {mode!r}")
    wal_common = [target_wal if x == target_m1 else x for x in common]
    wal_plan = [target_wal if x == target_m1 else x for x in plan]
    run(binary, ["scan"] + wal_common)
    run(binary, ["mask"] + wal_plan + ["--key-file", key_file])
    run(binary, ["approve"] + wal_plan + review)
    wal_before = sha256(target_wal)
    run_fail(
        binary,
        ["mask"] + wal_plan + ["--key-file", key_file] + review[:4] + ["--approved", "--apply"],
        "operation refused or incomplete",
    )
    if sha256(target_wal) != wal_before:
        raise SystemExit("FAIL: refused WAL apply modified the target main file")
    if emails(target_wal) != emails(source):
        raise SystemExit("FAIL: refused WAL apply changed target rows")
    print("WAL target refused before write; target main file and rows unchanged")

    print("== M2 streaming baseline and recovery schema ==")
    target_m2 = os.path.join(work, "target-m2.db")
    shutil.copyfile(source, target_m2)
    stream = [
        "--source", source, "--target", target_m2,
        "--strategy", "fake_email", "--key-id", "smoke-key",
        "--min-samples", "1", "--sample-limit", "10000", "--scope", "people.email",
        "--batch-size", "1",
    ]
    budget = os.path.join(work, "budget.json")
    with open(budget, "w", encoding="utf-8") as f:
        json.dump({
            "MaxBatchSize": 16, "MaxWorkers": 8, "MaxRowBytes": 1048576, "MaxColumns": 16,
            "MaxTables": 4, "MaxHistoryEntries": 128, "MaxHistoryNameBytes": 4096,
            "MaxReceiptPage": 32, "MaxIssueCounts": 16, "MaxQueue": 0,
            "MaxProcessBytes": 1073741824, "MaxDiskBytes": 1073741824,
        }, f)
    run(binary, ["init-outbox", "--target", target_m2])
    run(binary, ["init-recovery", "--target", target_m2])
    run(binary, ["approve"] + stream + review + ["--streaming"])
    baseline_run = "11111111111111111111111111111111"
    run(binary, ["mask"] + stream + ["--key-file", key_file] + review[:4] + [
        "--approved", "--apply", "--streaming", "--budget-file", budget,
        "--run-id", baseline_run, "--receipt-page-size", "8", "--timeout", "10m",
    ])
    baseline_emails = emails(target_m2)

    print("== M2 interrupted streaming apply (real SIGKILL after committed progress) ==")
    target_resume = os.path.join(work, "target-resume.db")
    shutil.copyfile(source, target_resume)
    resume_stream = [x if x != target_m2 else target_resume for x in stream]
    run(binary, ["init-outbox", "--target", target_resume])
    run(binary, ["init-recovery", "--target", target_resume])
    run(binary, ["approve"] + resume_stream + review + ["--streaming"])
    run_id = "22222222222222222222222222222222"
    apply_args = [
        binary, "mask",
    ] + resume_stream + [
        "--key-file", key_file,
    ] + review[:4] + [
        "--approved", "--apply", "--streaming", "--budget-file", budget,
        "--run-id", run_id, "--receipt-page-size", "8", "--timeout", "10m",
    ]
    proc = subprocess.Popen(apply_args, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    observed = 0
    deadline = time.time() + 60
    while time.time() < deadline:
        try:
            state, _ = recovery_state(target_resume, run_id)
        except sqlite3.Error:
            state = None
        if state and state[0] and state[0] >= 2:
            observed = state[0]
            break
        if proc.poll() is not None:
            break
        time.sleep(0.005)
    if proc.poll() is not None:
        raise SystemExit(f"FAIL: streaming apply finished before interruption (rc={proc.returncode})")
    proc.send_signal(signal.SIGKILL)
    returncode = proc.wait()
    if observed < 1 or returncode != -signal.SIGKILL:
        raise SystemExit(f"FAIL: interruption not observed (checkpoint={observed} rc={returncode})")
    print(f"observed checkpoint_sequence={observed} before SIGKILL")

    status = run(binary, ["status", "--target", target_resume, "--run-id", run_id])
    print(status.strip().splitlines()[0])
    run(binary, ["mask"] + resume_stream + ["--key-file", key_file] + review[:4] + [
        "--approved", "--apply", "--streaming", "--resume", "--budget-file", budget,
        "--run-id", run_id, "--receipt-page-size", "8", "--timeout", "10m",
    ])
    run(binary, ["validate"] + resume_stream + ["--key-file", key_file, "--strict"])
    if emails(target_resume) != baseline_emails:
        raise SystemExit("FAIL: resumed target differs from uninterrupted streaming baseline")
    before = sha256(target_resume)
    state_before, receipts_before = recovery_state(target_resume, run_id)
    run(binary, ["mask"] + resume_stream + ["--key-file", key_file] + review[:4] + [
        "--approved", "--apply", "--streaming", "--resume", "--budget-file", budget,
        "--run-id", run_id, "--receipt-page-size", "8", "--timeout", "10m",
    ])
    state_after, receipts_after = recovery_state(target_resume, run_id)
    if sha256(target_resume) != before or state_before != state_after or receipts_before != receipts_after:
        raise SystemExit("FAIL: second resume was not idempotent")
    if state_after[2] != 1 or state_after[3] != "passed":
        raise SystemExit(f"FAIL: recovery did not finish complete/passed: {state_after}")
    if sha256(source) != source_before:
        raise SystemExit("FAIL: source changed during recovery workflow")

    print("PASS: release binary M1 + M2 streaming + interruption + status + resume + validate + idempotent resume")
    print(f"archive input package: {os.path.basename(args.package)}")


if __name__ == "__main__":
    main()
