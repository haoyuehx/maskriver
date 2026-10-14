#!/usr/bin/env python3
"""Summarize actual synthetic SQLite Runner end-to-end benchmark runs.

Every sample includes opening readers, planning, preview, SQL writes and
independent strict validation. Reset/fixture creation is outside timing.
CPU/RSS is for the WHOLE compiled benchmark process, including fixture setup.
"""
import csv
import json
import re
import statistics
import sys
from pathlib import Path

BENCH = re.compile(r"^BenchmarkSQLiteM1Workflow/rows_(128|512|2048)/(short|long)/workers_(1|4)-\d+\s+(\d+)\s+([\d.]+) ns/op\b")
METRIC = re.compile(r"([\d.]+) rows/s")


def system_stat(text, title):
    m = re.search(r"^\s*" + re.escape(title) + r":\s*([\d.]+)\s*$", text, re.MULTILINE)
    if m is None:
        raise ValueError("missing real benchmark process measurement: " + title)
    return float(m.group(1))


def summarize_samples(values, rows, shape, workers):
    times = [v["ns_per_workflow"] for v in values]
    throughputs = [v["rows_per_second"] for v in values]
    return {
        "rows": rows,
        "distribution": shape,
        "workers": workers,
        "samples": values,
        "repetitions": len(values),
        "median_ns_per_workflow": statistics.median(times),
        "min_ns_per_workflow": min(times),
        "max_ns_per_workflow": max(times),
        "median_rows_per_second": statistics.median(throughputs),
        "min_rows_per_second": min(throughputs),
        "max_rows_per_second": max(throughputs),
    }


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: db_bench_report.py directory")
    folder = Path(sys.argv[1])
    raw = (folder / "sqlite-raw.txt").read_text(encoding="utf-8")
    groups = {}
    for line in raw.splitlines():
        match = BENCH.match(line.strip())
        if not match:
            continue
        rows, shape, workers, iterations, ns = match.groups()
        throughput = METRIC.search(line)
        if throughput is None:
            raise SystemExit("missing measured rows/s in real SQLite benchmark line")
        key = (int(rows), shape, int(workers))
        groups.setdefault(key, []).append({
            "iterations": int(iterations),
            "ns_per_workflow": float(ns),
            "rows_per_second": float(throughput.group(1)),
        })
    expected = {(count, shape, workers)
                for count in (128, 512, 2048)
                for shape in ("short", "long")
                for workers in (1, 4)}
    if set(groups) != expected or any(len(v) != 3 or any(row["iterations"] != 1 for row in v)
                                      for v in groups.values()):
        raise SystemExit("missing, unexpected or incomplete real SQLite benchmark measurements")

    summary = [summarize_samples(groups[key], *key) for key in sorted(groups)]
    proc_text = (folder / "sqlite-process.txt").read_text(encoding="utf-8")
    process = {
        "peak_rss_kb": int(system_stat(proc_text, "Maximum resident set size (kbytes)")),
        "user_cpu_seconds": system_stat(proc_text, "User time (seconds)"),
        "system_cpu_seconds": system_stat(proc_text, "System time (seconds)"),
    }
    result = {
        "method": "compiled Go test binary; -benchtime=1x, -count=3; measured Reader->Plan->Preview->Apply->strict Validate; disposable file resets excluded from Go timing",
        "limitations": "Only SQLite single-table/single-column synthetic workload; process CPU/RSS includes every benchmark and fixture setup. No production, MySQL CLI, steady-state RSS, end-user latency SLA or linear speedup claim.",
        "process": process,
        "measurements": summary,
    }
    (folder / "sqlite-results.json").write_text(json.dumps(result, indent=2), encoding="utf-8")
    fields = ["rows", "distribution", "workers", "repetitions",
              "median_ns_per_workflow", "min_ns_per_workflow",
              "max_ns_per_workflow", "median_rows_per_second",
              "min_rows_per_second", "max_rows_per_second"]
    with (folder / "sqlite-results.csv").open("w", encoding="utf-8", newline="") as stream:
        writer = csv.DictWriter(stream, fieldnames=fields)
        writer.writeheader()
        for row in summary:
            writer.writerow({k: row[k] for k in fields})
    # Static SVG drawn only from measured values, never an invented speedup.
    bars = []
    maximum = max(row["median_rows_per_second"] for row in summary) or 1
    for i, item in enumerate(summary):
        y = 76 + 24 * i
        width = item["median_rows_per_second"] * 400 / maximum
        label = f'{item["rows"]} {item["distribution"]} w{item["workers"]}'
        bars.append(
            f'<text x="12" y="{y+11}" font-size="11">{label}</text>'
            f'<rect x="165" y="{y}" width="{width:.1f}" height="14" fill="#516b83"/>'
            f'<text x="{175+width:.1f}" y="{y+11}" font-size="10">{item["median_rows_per_second"]:.1f}</text>'
        )
    svg = ('<svg xmlns="http://www.w3.org/2000/svg" width="760" height="420">'
           '<rect width="100%" height="100%" fill="white"/>'
           '<text x="12" y="25" font-size="16">Real SQLite synthetic workflow: median rows/s</text>'
           '<text x="12" y="43" font-size="11">3 runs per configuration; faster is higher</text>'
           + "".join(bars) +
           '<text x="12" y="396" font-size="11">Results depend on ephemeral CI CPU and disk.</text></svg>')
    (folder / "sqlite-summary.svg").write_text(svg, encoding="utf-8")
    for record in summary:
        print(record["rows"], record["distribution"], record["workers"],
              "median rows/s:", record["median_rows_per_second"],
              "range:", record["min_rows_per_second"], record["max_rows_per_second"])
    print("full benchmark process peak RSS KB:", process["peak_rss_kb"])


if __name__ == "__main__":
    main()
