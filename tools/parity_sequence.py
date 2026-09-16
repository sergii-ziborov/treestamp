"""Cache and watch-plan sequences against the pinned Rust oracle."""

from __future__ import annotations

from pathlib import Path
from typing import Any, Callable

DriverCall = Callable[..., dict[str, Any]]
Normalize = Callable[[dict[str, Any], str], Any]


def run_cache_watch_sequence(
    go_binary: Path,
    rust_binary: Path,
    corpus: Path,
    call: DriverCall,
    normalize: Normalize,
    canonical: Callable[[Any], str],
) -> dict[str, Any]:
    go_session = corpus.parent / "go-session.json"
    rust_session = corpus.parent / "rust-session.json"
    results: dict[str, Any] = {}
    compare_step(
        "scan_session",
        call(go_binary, "scan", corpus, session=go_session),
        call(rust_binary, "scan", corpus, session=rust_session),
        "scan",
        normalize,
        canonical,
        results,
    )
    reuse_go = call(go_binary, "scan_cached", corpus, session=go_session)
    reuse_rust = call(rust_binary, "scan_cached", corpus, session=rust_session)
    compare_step("scan_cached_reuse", reuse_go, reuse_rust, "scan", normalize, canonical, results)
    require_reuse(reuse_go, reuse_rust)
    before = file_hash(normalize(reuse_go, "scan"), "a.txt")
    (corpus / "a.txt").write_text("alpha-edited\n", encoding="utf-8", newline="\n")
    edit_go = call(go_binary, "scan_watch", corpus, session=go_session, plan={"changed": ["a.txt"]})
    compare_step(
        "scan_watch_edit",
        edit_go,
        call(rust_binary, "scan_watch", corpus, session=rust_session, plan={"changed": ["a.txt"]}),
        "scan",
        normalize,
        canonical,
        results,
        expect_reason="Incremental",
    )
    after = file_hash(normalize(edit_go, "scan"), "a.txt")
    if before == after:
        raise AssertionError("edited a.txt kept the previous content hash")
    (corpus / "extra.txt").write_text("extra\n", encoding="utf-8", newline="\n")
    compare_step(
        "scan_watch_create",
        call(go_binary, "scan_watch", corpus, session=go_session, plan={"changed": ["extra.txt"]}),
        call(rust_binary, "scan_watch", corpus, session=rust_session, plan={"changed": ["extra.txt"]}),
        "scan",
        normalize,
        canonical,
        results,
        expect_reason="Incremental",
    )
    compare_step(
        "scan_watch_full",
        call(go_binary, "scan_watch", corpus, session=go_session, plan={"full_rescan": True}),
        call(rust_binary, "scan_watch", corpus, session=rust_session, plan={"full_rescan": True}),
        "scan",
        normalize,
        canonical,
        results,
        expect_reason="FullRescan:StructuralChange",
    )
    (corpus / "a.txt").write_text("alpha-again\n", encoding="utf-8", newline="\n")
    compare_step(
        "scan_incremental",
        call(go_binary, "scan_incremental", corpus, session=go_session),
        call(rust_binary, "scan_incremental", corpus, session=rust_session),
        "scan",
        normalize,
        canonical,
        results,
    )
    return results


def compare_step(
    name: str,
    go_payload: dict[str, Any],
    rust_payload: dict[str, Any],
    operation: str,
    normalize: Normalize,
    canonical: Callable[[Any], str],
    results: dict[str, Any],
    expect_reason: str | None = None,
) -> None:
    left = drop_cache(normalize(go_payload, operation))
    right = drop_cache(normalize(rust_payload, operation))
    if left != right:
        raise AssertionError(f"{name} parity failed\nGo: {canonical(left)}\nRust: {canonical(right)}")
    if expect_reason and left.get("watch_reason") != expect_reason:
        raise AssertionError(f"{name} watch_reason={left.get('watch_reason')} want {expect_reason}")
    results[name] = {"status": "PASS", "items": len(left.get("files") or [])}


def require_reuse(go_payload: dict[str, Any], rust_payload: dict[str, Any]) -> None:
    go_reused = ((go_payload.get("data") or {}).get("cache") or {}).get("reused_hashes", 0)
    rust_reused = ((rust_payload.get("data") or {}).get("cache") or {}).get("reused_hashes", 0)
    if go_reused != rust_reused or go_reused == 0:
        raise AssertionError(f"cache reuse mismatch go={go_reused} rust={rust_reused}")


def drop_cache(value: Any) -> Any:
    if isinstance(value, dict):
        out = dict(value)
        out.pop("cache", None)
        return out
    return value


def file_hash(scan: dict[str, Any], relative: str) -> str:
    for item in scan.get("files") or []:
        if item.get("relative") == relative:
            return str(item.get("content_hash") or "")
    raise AssertionError(f"missing {relative}")
