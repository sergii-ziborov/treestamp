#!/usr/bin/env python3
"""Audit Treestamp bootstrap documents. This is not a scanner test."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PINNED_COMMIT = "29c003a6ad541c9a10faf30505235375fa78b9d8"
PINNED_TREE = "108c59e66c90c3b649b3ff867360a6c0e9ff7f6e"
CONTRACT_IDS = [f"T{index:02d}" for index in range(1, 36)]
BENCH_IDS = [f"B{index:02d}" for index in range(1, 15)]
REQUIRED_DOCS = [
    "README.md",
    "AGENTS.md",
    "ARCHITECTURE.md",
    "CONFORMANCE.md",
    "COMPETITORS.md",
    "BENCHMARKS.md",
    "THREAT_MODEL.md",
    "RELEASE.md",
    "LICENSE",
    "go.mod",
]


def load_json(relative: str) -> dict:
    path = ROOT / relative
    with path.open(encoding="utf-8") as handle:
        return json.load(handle)


def fail(errors: list[str]) -> int:
    for item in errors:
        print(f"FAIL: {item}", file=sys.stderr)
    return 1


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--require-full",
        action="store_true",
        help="Demand a finished full port. Must fail until every contract is closed.",
    )
    args = parser.parse_args()
    errors: list[str] = []

    for relative in REQUIRED_DOCS:
        if not (ROOT / relative).is_file():
            errors.append(f"missing {relative}")

    pin = load_json("compat/pin.json")
    upstream = pin.get("upstream", {})
    if upstream.get("commit") != PINNED_COMMIT:
        errors.append(f"pin commit must be {PINNED_COMMIT}")
    if upstream.get("tree") != PINNED_TREE:
        errors.append(f"pin tree must be {PINNED_TREE}")
    if upstream.get("cargo_version") != "0.5.2":
        errors.append("pin cargo_version must be 0.5.2")
    if upstream.get("descriptor_version") != 2:
        errors.append("descriptor_version must be 2")
    if upstream.get("cache_format") != 2:
        errors.append("cache_format must be 2")
    if pin.get("go_minimum") != "1.25.0":
        errors.append("go_minimum must be 1.25.0")
    exports = pin.get("crate_root_exports", {})
    if exports.get("total_named") != 108:
        errors.append("crate-root export total must be 108")

    inventory = load_json("compat/inventory.json")
    always = inventory["crate_root_exports"]["always"]
    if len(always) != 107:
        errors.append(f"inventory always-exports must be 107, got {len(always)}")
    if inventory["crate_root_exports"]["feature_gated"][0]["name"] != "RayonExecutor":
        errors.append("RayonExecutor must be the feature-gated crate-root export")
    if inventory["source"]["commit"] != PINNED_COMMIT:
        errors.append("inventory commit must match the pin")
    if inventory.get("member_count", 0) < 100:
        errors.append("inventory member_count looks empty")

    contracts = load_json("compat/contracts.json")
    ids = [item["id"] for item in contracts["contracts"]]
    if ids != CONTRACT_IDS:
        errors.append(f"contracts must be T01-T35 in order, got {ids}")

    ledger = load_json("compat/ledger.json")
    benches = ledger.get("benchmarks", {})
    for bench_id in BENCH_IDS:
        if bench_id not in benches:
            errors.append(f"ledger missing {bench_id}")

    cases = load_json("bench/cases.json")
    case_ids = [item["id"] for item in cases["cases"]]
    if case_ids != BENCH_IDS:
        errors.append(f"bench cases must be B01-B14, got {case_ids}")

    readme = (ROOT / "README.md").read_text(encoding="utf-8")
    for forbidden in (
        "crates.io",
        "fastest repository scanner",
        "264.7 ms",
        "1019.4 ms",
    ):
        if forbidden in readme:
            errors.append(f"README must not reuse Rust marketing or timings ({forbidden})")
    if "NOT_RUN" not in readme:
        errors.append("README must say benchmarks are NOT_RUN")
    if "scan" not in readme.lower() or "not implemented" not in readme.lower():
        errors.append("README must say the scanning API is not implemented")

    go_mod = (ROOT / "go.mod").read_text(encoding="utf-8")
    if "module github.com/sergii-ziborov/treestamp" not in go_mod:
        errors.append("go.mod module path is wrong")
    if "go 1.25.0" not in go_mod:
        errors.append("go.mod must declare go 1.25.0")
    if "github.com/fsnotify/fsnotify" in go_mod:
        errors.append("fsnotify must not be a main-module dependency")

    implemented = [
        item["id"]
        for item in contracts["contracts"]
        if item.get("status") == "IMPLEMENTED"
    ]
    if args.require_full:
        not_closed = [
            item["id"]
            for item in contracts["contracts"]
            if item.get("status") != "IMPLEMENTED"
        ]
        if not_closed:
            errors.append(
                "full port required but open contracts remain: " + ", ".join(not_closed)
            )
        if any(status != "MEASURED" for status in benches.values()):
            errors.append("full port required but benchmark campaign is incomplete")
        if pin.get("inventory_complete") is not True:
            errors.append("full port required but inventory_complete is not true")
        if errors:
            return fail(errors)
        print("FULL PORT CHECK PASSED")
        return 0

    if "T01" not in implemented:
        errors.append("P1 walker contract T01 should be IMPLEMENTED after the serial walker lands")

    if errors:
        return fail(errors)

    print("BOOTSTRAP AUDIT PASSED")
    print(f"pinned {PINNED_COMMIT}")
    print(f"contracts {len(ids)}; implemented {len(implemented)}")
    print("require-full would still fail: scanner, ignore, cache, and benches are open")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
