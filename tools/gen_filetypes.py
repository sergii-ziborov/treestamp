#!/usr/bin/env python3
"""Generate the named-type catalog from the pinned weavatrix-scan source."""

from __future__ import annotations

import argparse
import re
from pathlib import Path


def split_top_level(inner: str) -> list[str]:
    parts: list[str] = []
    start = 0
    depth = 0
    for index, char in enumerate(inner):
        if char == "[":
            depth += 1
        elif char == "]":
            depth -= 1
        elif char == "," and depth == 0:
            parts.append(inner[start:index])
            start = index + 1
    parts.append(inner[start:])
    return parts


def parse_catalog(text: str) -> list[tuple[list[str], list[str]]]:
    start = text.index("pub(crate) const DEFAULT_FILE_TYPES")
    block = text[start:]
    begin = block.index("[")
    end = block.rindex("];")
    body = block[begin + 1 : end]
    items: list[tuple[list[str], list[str]]] = []
    depth = 0
    item_start = None
    for index, char in enumerate(body):
        if char == "(" and depth == 0:
            item_start = index
        if char in "([":
            depth += 1
        elif char in ")]":
            depth -= 1
            if depth == 0 and item_start is not None:
                item = body[item_start : index + 1]
                names_m = re.search(r"&\[(.*?)\],\s*&\[(.*)\]\)$", item, re.S)
                if names_m:
                    names = re.findall(r'"((?:\\.|[^"])*)"', names_m.group(1))
                    pats = re.findall(r'"((?:\\.|[^"])*)"', names_m.group(2))
                    names = [bytes(n, "utf-8").decode("unicode_escape") if "\\" in n else n for n in names]
                    pats = [bytes(p, "utf-8").decode("unicode_escape") if "\\" in p else p for p in pats]
                    items.append((names, pats))
                item_start = None
    return items


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--src", required=True)
    parser.add_argument("--out", required=True)
    args = parser.parse_args()
    items = parse_catalog(Path(args.src).read_text(encoding="utf-8"))
    lines = [
        "package filetypes",
        "",
        "// Code generated from weavatrix-scan default_file_types.rs at 29c003a6.",
        "// Do not edit by hand.",
        "",
        "type typeDef struct {",
        "\tnames    []string",
        "\tpatterns []string",
        "}",
        "",
        "var catalog = []typeDef{",
    ]
    names_n = 0
    pats_n = 0
    for names, pats in items:
        names_n += len(names)
        pats_n += len(pats)
        nlit = ", ".join(go_string(name) for name in names)
        plit = ", ".join(go_string(pat) for pat in pats)
        lines.append(f"\t{{[]string{{{nlit}}}, []string{{{plit}}}}},")
    lines.append("}")
    lines.append("")
    Path(args.out).parent.mkdir(parents=True, exist_ok=True)
    Path(args.out).write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(f"names={names_n} patterns={pats_n} groups={len(items)}")
    if names_n != 265 or pats_n != 678:
        raise SystemExit(f"catalog mismatch names={names_n} patterns={pats_n}")
    return 0


def go_string(value: str) -> str:
    escaped = value.replace("\\", "\\\\").replace('"', '\\"')
    return f'"{escaped}"'


if __name__ == "__main__":
    raise SystemExit(main())
