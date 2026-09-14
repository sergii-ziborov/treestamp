#!/usr/bin/env python3
"""Extract the public surface of a pinned weavatrix-scan commit."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

CRATE_ROOT_ALWAYS = [
    "SCAN_CACHE_FORMAT_VERSION",
    "ScanCache",
    "ScanCacheEntry",
    "CacheValidationPolicy",
    "ContentDiscoveryMode",
    "ContentValidationPolicy",
    "EvidenceMode",
    "IgnorePolicy",
    "ScanLimits",
    "ScanOptions",
    "StandardSkips",
    "ChangedContentVisitOutcome",
    "ChangedContentVisitReport",
    "ContentFile",
    "ContentFileStatus",
    "ContentVisitControl",
    "ContentVisitEvent",
    "ContentVisitMode",
    "ContentVisitReport",
    "MultiContentVisitReport",
    "CancellationToken",
    "DeltaQuality",
    "ModifiedFile",
    "RenamedFile",
    "ScanDelta",
    "Error",
    "Result",
    "NamedFileTypes",
    "IgnoreFile",
    "RepositoryMatch",
    "RepositoryMatcher",
    "MultiScanReport",
    "MultiScanner",
    "ParallelVisitReport",
    "ParallelWalkIter",
    "ParallelWalkReport",
    "ParallelWalker",
    "WalkControl",
    "WalkEvent",
    "ParallelMultiVisitReport",
    "ParallelMultiWalkEvent",
    "ParallelMultiWalkReport",
    "ParallelMultiWalker",
    "collapse_path_prefixes",
    "is_same_or_descendant",
    "path_covered_by_prefixes",
    "PortableIgnoreSourceEvidence",
    "PortableScanReport",
    "PortableScanWarning",
    "PortableScannedFile",
    "PortableSkippedEntry",
    "CompactContentEvidence",
    "CompactScanReport",
    "CompactScannedFile",
    "FileIdentity",
    "FileVersion",
    "IgnoreSourceEvidence",
    "IgnoreSourceKind",
    "ScanCacheStats",
    "ScanReport",
    "ScanTermination",
    "ScanWarning",
    "ScannedFile",
    "SkipKind",
    "SkippedEntry",
    "ParallelExecutor",
    "ParallelJob",
    "ParallelRuntime",
    "SCAN_DESCRIPTOR_VERSION",
    "ScanDescriptor",
    "ScanSink",
    "ScanSinkControl",
    "ScanStreamReport",
    "Scanner",
    "scan_repository",
    "scan_repository_compact",
    "scan_repository_paths",
    "SelectionDecision",
    "SelectionDisposition",
    "SelectionMatcher",
    "ScanSession",
    "SnapshotContent",
    "SnapshotContentProvider",
    "SnapshotEvidence",
    "SnapshotReadError",
    "ParallelStatefulWalker",
    "StatefulWalkBuilder",
    "StatefulWalkEntry",
    "StatefulWalker",
    "ScanSummary",
    "MultiWalker",
    "WalkBuilder",
    "ErrorPolicy",
    "RootSymlinkPolicy",
    "WalkEntry",
    "WalkError",
    "WalkOperation",
    "WalkOptions",
    "WalkSkipReason",
    "Walker",
    "WatchEvent",
    "WatchEventKind",
    "WatchPlan",
    "WatcherEventAdapter",
    "FullRescanReason",
    "WatchUpdate",
    "WatchUpdateReason",
]

# src/lib.rs has 108 crate-root exports: 107 always visible plus RayonExecutor
# behind feature = "rayon".
FEATURE_EXPORTS = [
    {
        "name": "RayonExecutor",
        "feature": "rayon",
        "kind": "type",
    }
]
CRATE_ROOT_ALL = CRATE_ROOT_ALWAYS + [item["name"] for item in FEATURE_EXPORTS]


ITEM_RE = re.compile(
    r"""(?P<cfg>(?:#\[cfg\([^\]]+\)\]\s*)*)"""
    r"""(?P<vis>pub(?:\s*\([^)]+\))?)\s+"""
    r"""(?:(?P<unsafety>unsafe)\s+)?"""
    r"""(?:(?P<asyncness>async)\s+)?"""
    r"""(?P<kind>struct|enum|trait|type|const|static|fn|mod|use)\s+"""
    r"""(?P<name>[A-Za-z_][A-Za-z0-9_]*)""",
    re.MULTILINE,
)

IMPL_RE = re.compile(
    r"impl(?:<[^>]+>)?\s+(?:(?P<trait>[A-Za-z0-9_:]+)\s+for\s+)?(?P<type>[A-Za-z0-9_]+)",
)

ENUM_VARIANT_RE = re.compile(r"^\s*([A-Z][A-Za-z0-9_]*)\b", re.MULTILINE)
STRUCT_FIELD_RE = re.compile(
    r"^\s*pub(?:\s*\([^)]+\))?\s+([A-Za-z_][A-Za-z0-9_]*)\s*:",
    re.MULTILINE,
)
METHOD_RE = re.compile(
    r"^\s*(?:#\[(?!cfg)[^\]]+\]\s*)*pub(?:\s*\([^)]+\))?\s+(?:const\s+|async\s+|unsafe\s+)*fn\s+([A-Za-z_][A-Za-z0-9_]*)",
    re.MULTILINE,
)


def git_show(repo: Path, commit: str, relpath: str) -> str:
    return subprocess.check_output(
        ["git", "show", f"{commit}:{relpath}"],
        cwd=repo,
        text=True,
        encoding="utf-8",
    )


def git_ls(repo: Path, commit: str) -> list[str]:
    output = subprocess.check_output(
        ["git", "ls-tree", "-r", "--name-only", commit, "--", "src/"],
        cwd=repo,
        text=True,
        encoding="utf-8",
    )
    return [line.replace("\\", "/") for line in output.splitlines() if line.endswith(".rs")]


def strip_comments(source: str) -> str:
    source = re.sub(r"/\*.*?\*/", "", source, flags=re.DOTALL)
    return "\n".join(
        line for line in source.splitlines() if not line.strip().startswith("//")
    )


def feature_from_cfg(cfg: str) -> str | None:
    match = re.search(r'feature\s*=\s*"([^"]+)"', cfg)
    return match.group(1) if match else None


def extract_file(relpath: str, source: str) -> list[dict]:
    items: list[dict] = []
    cleaned = strip_comments(source)
    impl_type = None
    brace_depth = 0
    pending_impl = None

    lines = cleaned.splitlines()
    buffer: list[str] = []
    i = 0
    while i < len(lines):
        line = lines[i]
        buffer.append(line)
        brace_depth += line.count("{") - line.count("}")
        impl_match = IMPL_RE.search(line)
        if impl_match and "fn " not in line:
            pending_impl = impl_match.group("type")
        if "{" in line and pending_impl is not None:
            impl_type = pending_impl
            pending_impl = None
        if brace_depth <= 0:
            impl_type = None
            pending_impl = None
        i += 1

    for match in ITEM_RE.finditer(cleaned):
        kind = match.group("kind")
        name = match.group("name")
        vis = match.group("vis")
        if vis != "pub":
            continue
        if kind == "use":
            continue
        feature = feature_from_cfg(match.group("cfg") or "")
        items.append(
            {
                "name": name,
                "kind": kind,
                "path": relpath,
                "feature": feature,
                "impl_type": None,
            }
        )

    # Methods and inherent impls: scan impl blocks more directly.
    impl_blocks = re.split(r"\bimpl\b", cleaned)
    current_impl = None
    for block in impl_blocks[1:]:
        header, _, body = block.partition("{")
        type_match = re.search(
            r"(?:[A-Za-z0-9_:]+)\s+for\s+([A-Za-z0-9_]+)|([A-Za-z0-9_]+)",
            header,
        )
        if not type_match:
            continue
        current_impl = type_match.group(1) or type_match.group(2)
        for method in METHOD_RE.finditer(body):
            items.append(
                {
                    "name": method.group(1),
                    "kind": "method",
                    "path": relpath,
                    "feature": None,
                    "impl_type": current_impl,
                }
            )

    enum_blocks = re.finditer(
        r"pub\s+enum\s+([A-Za-z_][A-Za-z0-9_]*)[^{]*\{",
        cleaned,
    )
    for enum_match in enum_blocks:
        name = enum_match.group(1)
        start = enum_match.end()
        depth = 1
        end = start
        while end < len(cleaned) and depth:
            if cleaned[end] == "{":
                depth += 1
            elif cleaned[end] == "}":
                depth -= 1
            end += 1
        body = cleaned[start : end - 1]
        for variant in ENUM_VARIANT_RE.findall(body):
            if variant in {"Ok", "Err", "Some", "None", "Self"}:
                continue
            items.append(
                {
                    "name": variant,
                    "kind": "variant",
                    "path": relpath,
                    "feature": None,
                    "impl_type": name,
                }
            )

    struct_blocks = re.finditer(
        r"pub\s+struct\s+([A-Za-z_][A-Za-z0-9_]*)[^{;]*\{",
        cleaned,
    )
    for struct_match in struct_blocks:
        name = struct_match.group(1)
        start = struct_match.end()
        depth = 1
        end = start
        while end < len(cleaned) and depth:
            if cleaned[end] == "{":
                depth += 1
            elif cleaned[end] == "}":
                depth -= 1
            end += 1
        body = cleaned[start : end - 1]
        for field in STRUCT_FIELD_RE.findall(body):
            items.append(
                {
                    "name": field,
                    "kind": "field",
                    "path": relpath,
                    "feature": None,
                    "impl_type": name,
                }
            )
    return items


def crate_root_exports(lib_source: str) -> list[dict]:
    names: list[dict] = []
    feature = None
    for raw_line in lib_source.splitlines():
        line = raw_line.strip()
        cfg = re.search(r'#\[cfg\(feature\s*=\s*"([^"]+)"\)\]', line)
        if cfg:
            feature = cfg.group(1)
            continue
        if line.startswith("pub use"):
            exported = re.findall(r"[A-Za-z_][A-Za-z0-9_]*", line.split("use", 1)[1])
            skip = {"cfg", "feature", "rayon", "serde", "notify"}
            for name in exported:
                if name in skip:
                    continue
                if name[0].islower() and name not in {
                    "collapse_path_prefixes",
                    "is_same_or_descendant",
                    "path_covered_by_prefixes",
                    "scan_repository",
                    "scan_repository_compact",
                    "scan_repository_paths",
                }:
                    # module path segments such as cache::ScanCache
                    continue
                if name[0].isupper() or name in {
                    "collapse_path_prefixes",
                    "is_same_or_descendant",
                    "path_covered_by_prefixes",
                    "scan_repository",
                    "scan_repository_compact",
                    "scan_repository_paths",
                    "SCAN_CACHE_FORMAT_VERSION",
                    "SCAN_DESCRIPTOR_VERSION",
                }:
                    names.append({"name": name, "feature": feature})
            if not line.endswith("{") and not line.endswith(","):
                feature = None
        elif line == "};" or line == "}":
            feature = None
    return names


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--out", required=True)
    args = parser.parse_args()
    repo = Path(args.repo)
    commit = args.commit
    files = git_ls(repo, commit)
    lib_source = git_show(repo, commit, "src/lib.rs")
    parsed_exports = crate_root_exports(lib_source)
    export_names = [item["name"] for item in parsed_exports if not item["feature"]]
    if export_names != CRATE_ROOT_ALWAYS:
        missing = [name for name in CRATE_ROOT_ALWAYS if name not in export_names]
        extra = [name for name in export_names if name not in CRATE_ROOT_ALWAYS]
        print("parsed crate-root exports diverge from the pinned 108-name list", file=sys.stderr)
        print("missing", missing, file=sys.stderr)
        print("extra", extra, file=sys.stderr)
        print("parsed_count", len(export_names), file=sys.stderr)

    members: list[dict] = []
    for relpath in files:
        source = git_show(repo, commit, relpath)
        for item in extract_file(relpath, source):
            item["go_status"] = "NOT_IMPLEMENTED"
            if item["name"] in {
                "Walker",
                "WalkOptions",
                "WalkEntry",
                "WalkError",
                "WalkOperation",
                "WalkSkipReason",
                "ErrorPolicy",
                "RootSymlinkPolicy",
                "WalkBuilder",
                "MultiWalker",
                "collapse_path_prefixes",
                "is_same_or_descendant",
                "path_covered_by_prefixes",
            } and item["kind"] != "variant":
                item["go_status"] = "P1"
            members.append(item)

    payload = {
        "source": {
            "crate": "weavatrix-scan",
            "commit": commit,
            "note": "Public-member inventory extracted from the pinned commit. Method/field/variant coverage is regex-based and must be reviewed before a full-port release.",
        },
        "crate_root_exports": {
            "always": CRATE_ROOT_ALWAYS,
            "count_always": len(CRATE_ROOT_ALWAYS),
            "feature_gated": FEATURE_EXPORTS,
        },
        "parsed_crate_root_exports": parsed_exports,
        "source_files": files,
        "members": members,
        "member_count": len(members),
    }
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
    print(
        f"wrote {out} members={len(members)} files={len(files)} "
        f"always={len(CRATE_ROOT_ALWAYS)} crate_root={len(CRATE_ROOT_ALL)}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
