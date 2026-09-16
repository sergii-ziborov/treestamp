#!/usr/bin/env python3
"""Check recipe catalog, example regions, and local doc links."""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def load_catalog() -> dict:
    return json.loads((ROOT / "docs" / "catalog.json").read_text(encoding="utf-8"))


def example_source() -> str:
    return (ROOT / "dx" / "example_docs_test.go").read_text(encoding="utf-8")


def region(source: str, name: str) -> str:
    start = f"//region {name}\n"
    end = "//endregion"
    i = source.find(start)
    if i < 0:
        raise SystemExit(f"missing region {name}")
    j = source.find(end, i)
    if j < 0:
        raise SystemExit(f"unclosed region {name}")
    return source[i:j]


def check_structure() -> list[str]:
    errors: list[str] = []
    catalog = load_catalog()
    source = example_source()
    names = set()
    for item in catalog["examples"]:
        names.add(item["name"])
        if not (ROOT / item["recipe"]).is_file():
            errors.append(f"missing recipe {item['recipe']}")
        blob = region(source, item["region"])
        if f"func {item['name']}(" not in blob:
            errors.append(f"{item['name']} not in region {item['region']}")
        recipe = (ROOT / item["recipe"]).read_text(encoding="utf-8")
        if item["name"] not in recipe and item["name"] != "ExampleCompile":
            errors.append(f"{item['recipe']} does not name {item['name']}")
    for match in re.finditer(r"func (Example[A-Za-z0-9_]+)\(", source):
        if match.group(1) not in names:
            errors.append(f"uncatalogued {match.group(1)}")
    index = (ROOT / "docs" / "index.md").read_text(encoding="utf-8")
    for rel in (
        "choose-an-api.md",
        "troubleshooting.md",
        "tutorials/first-scan.md",
    ):
        if rel not in index:
            errors.append(f"index missing {rel}")
        if not (ROOT / "docs" / rel).is_file():
            errors.append(f"missing docs/{rel}")
    if not (ROOT / "examples" / "docquickstart" / "main.go").is_file():
        errors.append("missing examples/docquickstart/main.go")
    return errors


def parse_example_events(lines: list[str]) -> dict[str, str]:
    out: dict[str, str] = {}
    for raw in lines:
        raw = raw.strip()
        if not raw.startswith("{"):
            continue
        try:
            event = json.loads(raw)
        except json.JSONDecodeError:
            continue
        if event.get("Action") in {"pass", "fail", "skip"} and str(event.get("Test", "")).startswith("Example"):
            out[event["Test"]] = event["Action"]
    return out


def check_events(path: Path) -> list[str]:
    catalog = load_catalog()
    events = parse_example_events(path.read_text(encoding="utf-8").splitlines())
    errors: list[str] = []
    for item in catalog["examples"]:
        status = events.get(item["name"])
        if status is None:
            errors.append(f"example {item['name']} did not run")
        elif status != "pass":
            errors.append(f"example {item['name']} {status}")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--events", type=Path)
    args = parser.parse_args()
    errors = check_structure()
    if args.events:
        errors.extend(check_events(args.events))
    if errors:
        for item in errors:
            print(f"FAIL: {item}")
        return 1
    print("DOCS CHECK PASSED")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
