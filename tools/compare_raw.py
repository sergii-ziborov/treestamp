#!/usr/bin/env python3
"""Compare raw-walk JSON from two protocol drivers. Not a benchmark."""

from __future__ import annotations

import argparse
import json
import sys
from collections import Counter


def load(path: str) -> dict:
    with open(path, encoding="utf-8") as handle:
        return json.load(handle)


def normalize(entries: list[dict], ordered: bool) -> list[tuple] | Counter:
    items = []
    for item in entries:
        relative = (item.get("relative") or "").replace("\\", "/")
        items.append(
            (
                relative,
                item.get("depth"),
                bool(item.get("is_file")),
                bool(item.get("is_dir")),
                bool(item.get("is_symlink")),
                item.get("skip_reason"),
            )
        )
    if ordered:
        return items
    return Counter(items)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("left")
    parser.add_argument("right")
    parser.add_argument("--ordered", action="store_true")
    args = parser.parse_args()
    left = load(args.left)
    right = load(args.right)
    if left.get("error") or right.get("error"):
        print("error in payload", left.get("error"), right.get("error"), file=sys.stderr)
        return 2
    left_entries = left.get("data", {}).get("entries") or []
    right_entries = right.get("data", {}).get("entries") or []
    if normalize(left_entries, args.ordered) != normalize(right_entries, args.ordered):
        print("PARITY_FAIL", file=sys.stderr)
        print("left", len(left_entries), "right", len(right_entries), file=sys.stderr)
        return 1
    print("PARITY_OK", len(left_entries), "entries")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
