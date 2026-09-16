#!/usr/bin/env python3
"""Run method-matched informal benches and sample the test process RSS/CPU."""

from __future__ import annotations

import json
import os
import platform
import subprocess
import sys
import threading
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
COMPAT = ROOT / "bench" / "go-compat"


def sample_windows(pid: int, peaks: dict[str, float], stop: threading.Event) -> None:
    while not stop.wait(0.2):
        try:
            out = subprocess.check_output(
                [
                    "powershell",
                    "-NoProfile",
                    "-Command",
                    f"$p=Get-Process -Id {pid}; '{{0}} {{1}} {{2}}' -f $p.WorkingSet64,$p.CPU,$p.Handles",
                ],
                text=True,
            )
        except subprocess.CalledProcessError:
            return
        parts = out.split()
        if len(parts) < 2:
            continue
        ws = float(parts[0])
        cpu = float(parts[1])
        peaks["rss"] = max(peaks.get("rss", 0.0), ws)
        peaks["cpu"] = max(peaks.get("cpu", 0.0), cpu)


def run_benches() -> dict:
    env = os.environ.copy()
    env["CGO_ENABLED"] = "0"
    env["GOTOOLCHAIN"] = env.get("GOTOOLCHAIN", "local")
    env["GOWORK"] = "off"
    cmd = ["go", "test", "-bench", ".", "-benchmem", "-count", "3", "-timeout", "25m"]
    proc = subprocess.Popen(
        cmd,
        cwd=COMPAT,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
    )
    peaks: dict[str, float] = {}
    stop = threading.Event()
    sampler = None
    if os.name == "nt":
        sampler = threading.Thread(target=sample_windows, args=(proc.pid, peaks, stop), daemon=True)
        sampler.start()
    out, _ = proc.communicate()
    stop.set()
    if sampler:
        sampler.join(timeout=1)
    if proc.returncode != 0:
        sys.stderr.write(out)
        raise SystemExit(f"benches failed: {proc.returncode}")
    return {
        "output": out,
        "host": {
            "os": platform.system(),
            "arch": platform.machine(),
            "processor": platform.processor(),
            "python": platform.python_version(),
            "go": subprocess.check_output(["go", "env", "GOVERSION"], text=True, env=env).strip(),
        },
        "process": {
            "peak_working_set_bytes": int(peaks.get("rss", 0)),
            "peak_cpu_seconds": peaks.get("cpu", 0.0),
        },
    }


def parse_medians(text: str) -> list[dict]:
    rows: dict[str, list[dict]] = {}
    for line in text.splitlines():
        if not line.startswith("Benchmark"):
            continue
        parts = line.split()
        if len(parts) < 5:
            continue
        name = parts[0].rsplit("-", 1)[0]
        item = {
            "name": name,
            "ns_op": float(parts[2]),
            "b_op": int(parts[4]) if len(parts) > 4 else 0,
            "allocs_op": int(parts[6]) if len(parts) > 6 else 0,
        }
        rows.setdefault(name, []).append(item)
    out = []
    for name, items in rows.items():
        items.sort(key=lambda row: row["ns_op"])
        mid = items[len(items) // 2]
        out.append(mid)
    return out


def main() -> int:
    result = run_benches()
    result["medians"] = parse_medians(result["output"])
    dest = ROOT / "bench" / "go-compat" / "INFORMAL_RUN.json"
    dest.write_text(json.dumps(result, indent=2), encoding="utf-8")
    print(dest)
    print(f"peak_working_set_bytes={result['process']['peak_working_set_bytes']}")
    print(f"peak_cpu_seconds={result['process']['peak_cpu_seconds']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
