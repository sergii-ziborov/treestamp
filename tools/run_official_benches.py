#!/usr/bin/env python3
"""Record the official B01-B14 first campaign (1000-file tree)."""

from __future__ import annotations

import json
import os
import platform
import re
import subprocess
import sys
from datetime import date
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
RECEIPT = ROOT / "compat" / "results" / "official-benches.json"
LINE = re.compile(r"OFFICIAL (B\d{2}) MEASURED ns=(\d+)")


def go_version() -> str:
    completed = subprocess.run(
        ["go", "env", "GOVERSION"],
        cwd=ROOT,
        check=True,
        capture_output=True,
        text=True,
    )
    return completed.stdout.strip()


def main() -> int:
    env = os.environ.copy()
    env["TREESTAMP_OFFICIAL"] = "1"
    env["CGO_ENABLED"] = "0"
    completed = subprocess.run(
        ["go", "test", "./bench/official", "-count=1", "-v"],
        cwd=ROOT,
        check=False,
        capture_output=True,
        text=True,
        env=env,
    )
    text = completed.stdout + "\n" + completed.stderr
    print(text)
    if completed.returncode != 0:
        return completed.returncode
    cases: dict[str, dict] = {}
    for match in LINE.finditer(text):
        cases[match.group(1)] = {
            "status": "MEASURED",
            "ns": int(match.group(2)),
        }
    missing = [f"B{index:02d}" for index in range(1, 15) if f"B{index:02d}" not in cases]
    if missing:
        print(f"missing measurements: {', '.join(missing)}", file=sys.stderr)
        return 1
    RECEIPT.parent.mkdir(parents=True, exist_ok=True)
    RECEIPT.write_text(
        json.dumps(
            {
                "schema": 1,
                "campaign": "official-first",
                "size": 1000,
                "host": f"{platform.system()}/{platform.machine()}",
                "go": go_version(),
                "date": date.today().isoformat(),
                "note": (
                    "First official B01-B14 campaign on a 1000-file tree. "
                    "Not a 10k/100k/1M ranking. Informal 16 Sep Windows listing "
                    "medians stay in bench/go-compat/INFORMAL_RUN.json."
                ),
                "cases": cases,
            },
            indent=2,
        )
        + "\n",
        encoding="utf-8",
    )
    print(f"wrote {RECEIPT.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
