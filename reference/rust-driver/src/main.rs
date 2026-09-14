use serde::Deserialize;
use serde_json::{json, Value};
use std::io::{self, Read};
use weavatrix_scan::{
    ErrorPolicy, RootSymlinkPolicy, WalkBuilder, WalkOptions, WalkSkipReason, Walker,
};

#[derive(Debug, Deserialize)]
struct Request {
    op: String,
    root: String,
    #[serde(default)]
    options: DriverOptions,
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
    match request.op.as_str() {
        "raw_walk_serial" | "raw_walk_sorted" => {}
        "scan" | "scan_compact" | "scan_paths" => {
            emit_err("use weavatrix-scan directly for scan ops; this driver starts at raw walk");
            std::process::exit(2);
        }
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

fn collect<I>(walker: I) -> Result<Vec<Value>, String>
where
    I: Iterator<Item = Result<weavatrix_scan::WalkEntry, weavatrix_scan::WalkError>>,
{
    let mut entries = Vec::new();
    for item in walker {
        let entry = item.map_err(|error| error.to_string())?;
        let relative = entry
            .relative_path()
            .to_string_lossy()
            .replace('\\', "/");
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
