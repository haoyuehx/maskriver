#!/usr/bin/env python3
"""Summarize synthetic go test -benchmem measurements without selecting a best run."""
import csv
import json
import re
import statistics
import sys
from pathlib import Path

LINE = re.compile(r"^(BenchmarkMaskBatchWorkers/[^\s]+)\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op\s*$")


def main():
    if len(sys.argv) != 3:
        raise SystemExit("usage: bench_report.py input.txt output-dir")
    src = Path(sys.argv[1])
    output = Path(sys.argv[2])
    output.mkdir(parents=True, exist_ok=True)
    groups = {}
    for raw in src.read_text(encoding="utf-8").splitlines():
        match = LINE.match(raw.strip())
        if not match:
            continue
        name, ns, bytes_op, allocs = match.groups()
        groups.setdefault(name, []).append({
            "ns_per_op": float(ns),
            "bytes_per_op": int(bytes_op),
            "allocs_per_op": int(allocs),
        })
    if len(groups) != 4 or any(len(xs) != 3 for xs in groups.values()):
        raise SystemExit("expected 3 real benchmark samples for each of 4 worker counts")
    rows = []
    for name, samples in sorted(groups.items()):
        times = [item["ns_per_op"] for item in samples]
        rows.append({
            "benchmark": name,
            "runs": len(times),
            "median_ns_per_op": statistics.median(times),
            "minimum_ns_per_op": min(times),
            "maximum_ns_per_op": max(times),
            "median_bytes_per_op": statistics.median(item["bytes_per_op"] for item in samples),
            "median_allocs_per_op": statistics.median(item["allocs_per_op"] for item in samples),
            "samples": samples,
        })
    (output / "results.json").write_text(json.dumps({
        "note": "Synthetic 128-value HMAC transform batches; not full database throughput. No machine-independence, latency SLA, or speedup claim.",
        "results": rows,
    }, indent=2), encoding="utf-8")
    with (output / "results.csv").open("w", encoding="utf-8", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=[
            "benchmark", "runs", "median_ns_per_op", "minimum_ns_per_op",
            "maximum_ns_per_op", "median_bytes_per_op", "median_allocs_per_op",
        ])
        writer.writeheader()
        for row in rows:
            writer.writerow({k: row[k] for k in writer.fieldnames})
    scale = max(row["median_ns_per_op"] for row in rows) or 1
    bars = []
    for index, row in enumerate(rows):
        y = 67 + index * 56
        width = 410 * row["median_ns_per_op"] / scale
        label = row["benchmark"].split("/")[-1].split("-")[0]
        bars.append(
            f'<text x="16" y="{y+17}" font-size="14">{label}</text>'
            f'<rect x="135" y="{y}" width="{width:.1f}" height="23" fill="#405b78"/>'
            f'<text x="{145+width:.1f}" y="{y+17}" font-size="12">{row["median_ns_per_op"]:.0f} ns/op</text>'
        )
    svg = ('<svg xmlns="http://www.w3.org/2000/svg" width="770" height="350">'
           '<rect width="100%" height="100%" fill="white"/>'
           '<text x="16" y="30" font-size="18">Synthetic mask batch, median of 3 runs</text>'
           + "".join(bars) +
           '<text x="16" y="320" font-size="12">Lower is faster; all measurements depend on CI hardware.</text>'
           '</svg>')
    (output / "summary.svg").write_text(svg, encoding="utf-8")
    for row in rows:
        print(row["benchmark"], "median_ns/op", row["median_ns_per_op"],
              "min", row["minimum_ns_per_op"], "max", row["maximum_ns_per_op"])


if __name__ == "__main__":
    main()
