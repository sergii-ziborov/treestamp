use serde::Deserialize;
use serde_json::{Value, json};
use std::io::{self, Read};
use weavatrix_scan::{
    CompactScanReport, ErrorPolicy, IgnoreSourceKind, RootSymlinkPolicy, ScanReport, Scanner,
    ScanTermination, SkipKind, WalkBuilder, WalkOptions, WalkSkipReason, Walker, WatchPlan,
    scan_repository, scan_repository_compact, scan_repository_paths,
};

#[derive(Debug, Deserialize)]
struct Request {
    op: String,
    root: String,
    #[serde(default)]
    options: DriverOptions,
    #[serde(default)]
    session: Option<String>,
    #[serde(default)]
    plan: PlanOptions,
}

#[derive(Debug, Default, Deserialize)]
struct DriverOptions {
    #[serde(default)]
    min_depth: usize,
    max_depth: Option<usize>,
    #[serde(default)]
    max_open: Option<usize>,
    #[serde(default)]
    same_file_system: bool,
    #[serde(default)]
    follow_links: bool,
    #[serde(default)]
    collect_metadata: bool,
    #[serde(default)]
    error_policy: Option<String>,
    #[serde(default)]
    root_symlink_policy: Option<String>,
    #[serde(default)]
    sort_by_file_name: bool,
    #[serde(default)]
    contents_first: bool,
}

#[derive(Debug, Default, Deserialize)]
struct PlanOptions {
    #[serde(default)]
    changed: Vec<String>,
    #[serde(default)]
    removed: Vec<String>,
    #[serde(default)]
    full_rescan: bool,
    #[serde(default)]
    rejected_events: u64,
}

struct ReportParts<'a> {
    files: Vec<Value>,
    skipped: Vec<Value>,
    warnings: Vec<Value>,
    sources: Vec<Value>,
    revision: &'a str,
    descriptor_version: u32,
    descriptor_policy: &'a str,
    complete: bool,
    termination: Option<ScanTermination>,
    portable: bool,
    reused_hashes: u64,
    content_reads: u64,
    fingerprint_reads: u64,
}

fn main() {
    let mut raw = String::new();
    if let Err(error) = io::stdin().read_to_string(&mut raw) {
        emit_err(&error.to_string());
        std::process::exit(1);
    }
    let request: Request = match serde_json::from_str(&raw) {
        Ok(value) => value,
        Err(error) => {
            emit_err(&error.to_string());
            std::process::exit(1);
        }
    };
    if run_scan(&request) {
        return;
    }
    match request.op.as_str() {
        "raw_walk_serial" | "raw_walk_sorted" => {}
        other => {
            emit_err(&format!("unknown op {other}"));
            std::process::exit(1);
        }
    }
    let mut options = WalkOptions::default()
        .with_min_depth(request.options.min_depth)
        .with_max_depth(request.options.max_depth)
        .with_same_file_system(request.options.same_file_system)
        .with_follow_links(request.options.follow_links)
        .with_metadata(request.options.collect_metadata);
    if let Some(max_open) = request.options.max_open {
        options = options.with_max_open(max_open);
    }
    if request.options.error_policy.as_deref() == Some("abort") {
        options = options.with_error_policy(ErrorPolicy::Abort);
    }
    if request.options.root_symlink_policy.as_deref() == Some("reject") {
        options = options.with_root_symlink_policy(RootSymlinkPolicy::Reject);
    }
    let sorted = request.op == "raw_walk_sorted" || request.options.sort_by_file_name;
    let result = if sorted || request.options.contents_first {
        let mut builder = WalkBuilder::new(&request.root).options(options);
        if sorted {
            builder = builder.sort_by_file_name();
        }
        builder = builder.contents_first(request.options.contents_first);
        collect(builder.build())
    } else {
        match Walker::with_options(&request.root, options) {
            Ok(walker) => collect(walker),
            Err(error) => {
                emit_err(&error.to_string());
                std::process::exit(1);
            }
        }
    };
    match result {
        Ok(entries) => {
            let payload = json!({
                "data": { "entries": entries },
                "timing": Value::Null,
                "error": Value::Null
            });
            println!("{payload}");
        }
        Err(error) => {
            emit_err(&error);
            std::process::exit(1);
        }
    }
}

fn run_scan(request: &Request) -> bool {
    match request.op.as_str() {
        "scan" => emit_result(scan_and_store(request)),
        "scan_compact" => {
            emit_result(scan_repository_compact(&request.root).map(|report| compact_json(&report)))
        }
        "scan_paths" => {
            emit_result(scan_repository_paths(&request.root).map(|paths| json!({ "paths": paths })))
        }
        "scan_cached" => emit_result(scan_cached(request)),
        "scan_incremental" => emit_result(scan_incremental(request)),
        "scan_watch" => emit_result(scan_watch(request)),
        _ => return false,
    }
    true
}

fn scan_and_store(request: &Request) -> Result<Value, String> {
    let report = scan_repository(&request.root).map_err(|error| error.to_string())?;
    store_session(request, &report)?;
    Ok(scan_json(&report))
}

fn scan_cached(request: &Request) -> Result<Value, String> {
    let previous = load_session(request)?;
    let cache = previous.to_cache();
    let report = Scanner::new(&request.root)
        .scan_cached(&cache)
        .map_err(|error| error.to_string())?;
    store_session(request, &report)?;
    Ok(scan_json(&report))
}

fn scan_incremental(request: &Request) -> Result<Value, String> {
    let previous = load_session(request)?;
    let report = Scanner::new(&request.root)
        .scan_incremental(&previous)
        .map_err(|error| error.to_string())?;
    store_session(request, &report)?;
    Ok(scan_json(&report))
}

fn scan_watch(request: &Request) -> Result<Value, String> {
    let previous = load_session(request)?;
    let plan = WatchPlan {
        changed: request.plan.changed.clone(),
        removed: request.plan.removed.clone(),
        full_rescan: request.plan.full_rescan,
        rejected_events: request.plan.rejected_events,
    };
    let update = Scanner::new(&request.root)
        .scan_watch_plan_detailed(&previous, &plan)
        .map_err(|error| error.to_string())?;
    store_session(request, &update.report)?;
    let mut data = scan_json(&update.report);
    data["watch_reason"] = json!(update.reason.as_str());
    Ok(data)
}

fn load_session(request: &Request) -> Result<ScanReport, String> {
    let path = request.session.as_ref().ok_or_else(|| "session path required".to_string())?;
    let raw = std::fs::read(path).map_err(|error| error.to_string())?;
    serde_json::from_slice(&raw).map_err(|error| error.to_string())
}

fn store_session(request: &Request, report: &ScanReport) -> Result<(), String> {
    let Some(path) = &request.session else {
        return Ok(());
    };
    let raw = serde_json::to_vec(report).map_err(|error| error.to_string())?;
    std::fs::write(path, raw).map_err(|error| error.to_string())
}

fn emit_result<E: std::fmt::Display>(result: Result<Value, E>) {
    match result {
        Ok(data) => println!(
            "{}",
            json!({ "data": data, "timing": Value::Null, "error": Value::Null })
        ),
        Err(error) => {
            emit_err(&error.to_string());
            std::process::exit(1);
        }
    }
}

fn scan_json(report: &ScanReport) -> Value {
    let files = report
        .files
        .iter()
        .map(|file| {
            json!({
                "relative": file.relative,
                "bytes": file.bytes,
                "content_hash": file.content_hash,
                "content_fingerprint": file.content_fingerprint,
                "binary_checked": file.binary_checked,
            })
        })
        .collect::<Vec<_>>();
    report_json(ReportParts {
        files,
        skipped: report.skipped.iter().map(skip_json).collect(),
        warnings: report
            .warnings
            .iter()
            .map(|warning| {
                json!({
                    "relative": warning.relative,
                    "message": warning.message,
                })
            })
            .collect(),
        sources: report.ignore_sources.iter().map(source_json).collect(),
        revision: &report.revision,
        descriptor_version: report.descriptor.version,
        descriptor_policy: &report.descriptor.policy,
        complete: report.complete,
        termination: report.termination,
        portable: report.portable,
        reused_hashes: report.cache.reused_hashes,
        content_reads: report.cache.content_reads,
        fingerprint_reads: report.cache.fingerprint_reads,
    })
}

fn compact_json(report: &CompactScanReport) -> Value {
    let files = report.files.iter().map(|file| {
        let content = file.content.as_deref();
        json!({
            "relative": file.relative.as_ref(),
            "bytes": file.bytes,
            "content_hash": file.content_hash(),
            "content_fingerprint": content.and_then(|value| value.content_fingerprint.as_deref()),
            "binary_checked": content.is_some_and(|value| value.binary_checked),
        })
    }).collect::<Vec<_>>();
    report_json(ReportParts {
        files,
        skipped: report.skipped.iter().map(skip_json).collect(),
        warnings: report
            .warnings
            .iter()
            .map(|warning| {
                json!({
                    "relative": warning.relative,
                    "message": warning.message,
                })
            })
            .collect(),
        sources: report.ignore_sources.iter().map(source_json).collect(),
        revision: &report.revision,
        descriptor_version: report.descriptor.version,
        descriptor_policy: &report.descriptor.policy,
        complete: report.complete,
        termination: report.termination,
        portable: report.portable,
        reused_hashes: report.cache.reused_hashes,
        content_reads: report.cache.content_reads,
        fingerprint_reads: report.cache.fingerprint_reads,
    })
}

fn report_json(parts: ReportParts<'_>) -> Value {
    json!({
        "files": parts.files,
        "skipped": parts.skipped,
        "warnings": parts.warnings,
        "ignore_sources": parts.sources,
        "revision": parts.revision,
        "descriptor": { "version": parts.descriptor_version, "policy": parts.descriptor_policy },
        "complete": parts.complete,
        "termination": parts.termination.map(termination_label),
        "portable": parts.portable,
        "cache": {
            "reused_hashes": parts.reused_hashes,
            "content_reads": parts.content_reads,
            "fingerprint_reads": parts.fingerprint_reads,
        },
    })
}

fn skip_json(entry: &weavatrix_scan::SkippedEntry) -> Value {
    json!({
        "relative": entry.relative,
        "kind": scan_skip_label(entry.kind),
        "detail": entry.detail,
    })
}

fn source_json(source: &weavatrix_scan::IgnoreSourceEvidence) -> Value {
    json!({
        "kind": source_label(source.kind),
        "location": source.location,
        "content_hash": source.content_hash,
    })
}

fn scan_skip_label(kind: SkipKind) -> &'static str {
    match kind {
        SkipKind::Binary => "binary",
        SkipKind::FileSystemBoundary => "filesystem_boundary",
        SkipKind::Extension => "extension",
        SkipKind::Ignored => "ignored",
        SkipKind::IoError => "io_error",
        SkipKind::MaxDepth => "max_depth",
        SkipKind::Oversized => "oversized",
        SkipKind::PathEscape => "path_escape",
        SkipKind::StandardDirectory => "standard_directory",
        SkipKind::Hidden => "hidden",
        SkipKind::Override => "override",
        SkipKind::Symlink => "symlink",
        SkipKind::SymlinkLoop => "symlink_loop",
        SkipKind::ScanLimit => "scan_limit",
        SkipKind::ConcurrentModification => "concurrent_modification",
    }
}

fn source_label(kind: IgnoreSourceKind) -> &'static str {
    match kind {
        IgnoreSourceKind::GitGlobal => "git_global",
        IgnoreSourceKind::GitExclude => "git_exclude",
        IgnoreSourceKind::GitIgnore => "git_ignore",
        IgnoreSourceKind::DotIgnore => "dot_ignore",
        IgnoreSourceKind::Custom => "custom",
        IgnoreSourceKind::Explicit => "explicit",
        IgnoreSourceKind::Override => "override",
    }
}

fn termination_label(value: ScanTermination) -> &'static str {
    match value {
        ScanTermination::MaxEntries => "max_entries",
        ScanTermination::MaxTotalBytes => "max_total_bytes",
        ScanTermination::Timeout => "timeout",
        ScanTermination::Cancelled => "cancelled",
    }
}

fn collect<I>(walker: I) -> Result<Vec<Value>, String>
where
    I: Iterator<Item = Result<weavatrix_scan::WalkEntry, weavatrix_scan::WalkError>>,
{
    let mut entries = Vec::new();
    for item in walker {
        let entry = item.map_err(|error| error.to_string())?;
        let relative = entry.relative_path().to_string_lossy().replace('\\', "/");
        let skip = entry.skip_reason().map(skip_label);
        entries.push(json!({
            "relative": relative,
            "depth": entry.depth(),
            "is_file": entry.is_file(),
            "is_dir": entry.is_dir(),
            "is_symlink": entry.is_symlink(),
            "bytes": entry.bytes(),
            "skip_reason": skip,
        }));
    }
    Ok(entries)
}

fn skip_label(reason: WalkSkipReason) -> &'static str {
    match reason {
        WalkSkipReason::MaxDepth => "max_depth",
        WalkSkipReason::FileSystemBoundary => "filesystem_boundary",
        WalkSkipReason::PathEscape => "path_escape",
        WalkSkipReason::SymlinkLoop => "symlink_loop",
    }
}

fn emit_err(message: &str) {
    let payload = json!({
        "data": Value::Null,
        "timing": Value::Null,
        "error": message,
    });
    println!("{payload}");
}
