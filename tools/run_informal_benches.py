#!/usr/bin/env python3
"""Run method-matched informal benches and sample the compiled test process."""

from __future__ import annotations

import argparse
import json
import os
import platform
import subprocess
import sys
import threading
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
COMPAT = ROOT / "bench" / "go-compat"
MODULES = (
    "github.com/charlievieth/fastwalk",
    "github.com/boyter/gocodewalker",
    "github.com/karrick/godirwalk",
)


def sample_windows(pid: int, peaks: dict[str, float], stop: threading.Event) -> None:
    while not stop.wait(0.2):
        try:
            out = subprocess.check_output(
                [
                    "powershell",
                    "-NoProfile",
                    "-Command",
                    f"$p=Get-Process -Id {pid}; '{{0}} {{1}}' -f $p.WorkingSet64,$p.CPU",
                ],
                text=True,
            )
        except subprocess.CalledProcessError:
            return
        parts = out.split()
        if len(parts) < 2:
            continue
        peaks["rss"] = max(peaks.get("rss", 0.0), float(parts[0]))
        peaks["cpu"] = max(peaks.get("cpu", 0.0), float(parts[1]))


def env_with_local() -> dict[str, str]:
    env = os.environ.copy()
    env["CGO_ENABLED"] = "0"
    env["GOTOOLCHAIN"] = env.get("GOTOOLCHAIN", "local")
    env["GOWORK"] = "off"
    env["GOEXPERIMENT"] = ""
    return env


def run(cmd: list[str], env: dict[str, str], sample: bool) -> tuple[str, dict[str, float]]:
    proc = subprocess.Popen(
        cmd, cwd=COMPAT, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True
    )
    peaks: dict[str, float] = {}
    stop = threading.Event()
    sampler = None
    if sample and os.name == "nt":
        sampler = threading.Thread(target=sample_windows, args=(proc.pid, peaks, stop), daemon=True)
        sampler.start()
    out, _ = proc.communicate()
    stop.set()
    if sampler:
        sampler.join(timeout=1)
    if proc.returncode != 0:
        sys.stderr.write(out)
        raise SystemExit(f"command failed: {cmd} ({proc.returncode})")
    return out, peaks


def module_versions(env: dict[str, str]) -> dict[str, str]:
    out = subprocess.check_output(["go", "list", "-m"] + list(MODULES), cwd=COMPAT, env=env, text=True)
    versions = {}
    for line in out.splitlines():
        parts = line.split()
        if len(parts) >= 2:
            versions[parts[0]] = parts[1]
    return versions


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
        out.append(items[len(items) // 2])
    return out


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--walkdirs",
        action="store_true",
        help="WalkDirs-matched benches only. Writes WALKDIRS_G05.json, not INFORMAL_RUN.json.",
    )
    args = parser.parse_args()
    env = env_with_local()
    versions = module_versions(env)
    go = subprocess.check_output(["go", "env", "GOVERSION"], text=True, env=env).strip()
    bench = (
        "BenchmarkWalkDirs|BenchmarkGodirwalk|BenchmarkTreestampReadDir|BenchmarkTreestampDirScanner|BenchmarkNewDirent"
        if args.walkdirs
        else "."
    )
    timeout = "15m" if args.walkdirs else "25m"
    out, _ = run(["go", "test", "-bench", bench, "-benchmem", "-count", "3", "-timeout", timeout], env, False)
    peaks: dict[str, float] = {}
    command = f"go test -bench={bench} -benchmem -count=3"
    if not args.walkdirs:
        exe = "compat.test.exe" if os.name == "nt" else "compat.test"
        run(["go", "test", "-c", "-o", exe, "."], env, False)
        _, peaks = run(
            [str(COMPAT / exe), "-test.bench=.", "-test.benchmem", "-test.count=1", "-test.timeout=25m"],
            env,
            True,
        )
        command = f"go test -c -o {exe} . && ./{exe} -test.bench=. -test.benchmem -test.count=1"
    result = {
        "output": out,
        "host": {
            "os": platform.system(),
            "arch": platform.machine(),
            "processor": platform.processor(),
            "python": platform.python_version(),
            "go": go,
            "cgo": env.get("CGO_ENABLED", ""),
            "gotoolchain": env.get("GOTOOLCHAIN", ""),
        },
        "modules": versions,
        "process": {
            "note": "RSS/CPU are from the compiled test binary, not go test",
            "compiled_test_peak_working_set_bytes": int(peaks.get("rss", 0)),
            "compiled_test_cpu_seconds": peaks.get("cpu", 0.0),
            "compiled_test_command": command,
        },
        "medians": parse_medians(out),
        "official_benches": "NOT_RUN",
        "mixes_sorted_unsorted": False,
    }
    dest = COMPAT / ("WALKDIRS_G05.json" if args.walkdirs else "INFORMAL_RUN.json")
    dest.write_text(json.dumps(result, indent=2), encoding="utf-8")
    print(dest)
    print(f"modules={versions}")
    print(f"compiled_rss={result['process']['compiled_test_peak_working_set_bytes']}")
    print(f"compiled_cpu={result['process']['compiled_test_cpu_seconds']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
