#!/usr/bin/env python3
"""Record /usr/bin/time -v process metrics from synthetic worker benchmarks.

The process includes Go test startup and compilation/caching; RSS/CPU are not
pure masking-runtime measurements and may not be used as SLA claims.
"""
import csv
import json
import re
import sys
from pathlib import Path

PATTERN = re.compile(r"BenchmarkMaskBatchWorkers/workers_\d+-\d+\s+\d+\s+([\d.]+) ns/op")


def time_value(text, label):
    match = re.search(r"^\s*" + re.escape(label) + r":\s*([\d.]+)\s*$", text, re.MULTILINE)
    if not match:
        raise ValueError(f"missing measurement: {label}")
    return float(match.group(1))


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: bench_system_report.py bench-out")
    root = Path(sys.argv[1])
    rows = []
    for workers in (1, 2, 4, 8):
        output = (root / f"process-workers-{workers}-raw.txt").read_text(encoding="utf-8")
        metric = (root / f"process-workers-{workers}.txt").read_text(encoding="utf-8")
        ns_matches = PATTERN.findall(output)
        if len(ns_matches) != 1:
            raise SystemExit(f"missing or duplicate actual worker-{workers} benchmark")
        ns_op = float(ns_matches[0])
        if ns_op <= 0:
            raise SystemExit("invalid benchmark duration")
        rows.append({
            "workers": workers,
            "ns_per_batch_128": ns_op,
            "synthetic_values_per_second": 128 * 1e9 / ns_op,
            "process_peak_rss_kb": int(time_value(metric, "Maximum resident set size (kbytes)")),
            "process_user_cpu_seconds": time_value(metric, "User time (seconds)"),
            "process_system_cpu_seconds": time_value(metric, "System time (seconds)"),
            "process_elapsed": next((line.split(": ", 1)[-1].strip() for line in metric.splitlines()
                                      if "Elapsed (wall clock) time" in line), "unknown"),
        })
    manifest = {
        "method": "four separate synthetic go test process invocations, 500ms per worker setting",
        "limitations": "CPU seconds and peak RSS include Go test harness process startup, not pure masking runtime or database throughput",
        "row_count_per_batch": 128,
        "results": rows,
    }
    (root / "process-summary.json").write_text(json.dumps(manifest, indent=2), encoding="utf-8")
    with (root / "process-summary.csv").open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=[k for k in rows[0] if k != "process_elapsed"] + ["process_elapsed"])
        writer.writeheader()
        writer.writerows(rows)


if __name__ == "__main__":
    main()
