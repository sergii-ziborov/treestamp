package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/sergii-ziborov/treestamp"
)

func runSessionScan(req request) {
	switch req.Op {
	case "scan":
		report, err := treestamp.Scan(context.Background(), req.Root)
		writeScan(req.Session, report, err, "")
	case "scan_compact":
		report, err := treestamp.ScanCompact(context.Background(), req.Root)
		if err != nil {
			fail(err.Error())
		}
		write(response{Data: compactData(report)})
	case "scan_paths":
		paths, err := treestamp.ScanPaths(context.Background(), req.Root)
		if err != nil {
			fail(err.Error())
		}
		write(response{Data: map[string]any{"paths": paths}})
	case "scan_cached":
		cache := sessionCache(req.Session)
		report, err := scannerOf(req.Root).ScanCached(context.Background(), cache)
		writeScan(req.Session, report, err, "")
	case "scan_incremental":
		previous := mustSession(req.Session)
		report, err := scannerOf(req.Root).ScanIncremental(context.Background(), previous)
		writeScan(req.Session, report, err, "")
	case "scan_watch":
		previous := mustSession(req.Session)
		update, err := scannerOf(req.Root).ScanWatchPlanDetailed(context.Background(), previous, treestamp.WatchPlan{
			Changed: req.Plan.Changed, Removed: req.Plan.Removed,
			FullRescan: req.Plan.FullRescan, RejectedEvents: req.Plan.RejectedEvents,
		})
		if err != nil {
			fail(err.Error())
		}
		writeScan(req.Session, update.Report, nil, update.Reason.String())
	}
}

func scannerOf(root string) *treestamp.Scanner {
	scanner, err := treestamp.NewScanner(root)
	if err != nil {
		fail(err.Error())
	}
	return scanner
}

func writeScan(session string, report *treestamp.ScanReport, err error, reason string) {
	if err != nil {
		fail(err.Error())
	}
	if session != "" {
		if saveErr := saveSession(session, report); saveErr != nil {
			fail(saveErr.Error())
		}
	}
	data := scanData(report)
	if reason != "" {
		data.WatchReason = &reason
	}
	write(response{Data: data})
}

func mustSession(path string) *treestamp.ScanReport {
	report, err := loadSession(path)
	if err != nil {
		fail(err.Error())
	}
	return report
}

func sessionCache(path string) *treestamp.ScanCache {
	report := mustSession(path)
	cache := report.ToCache()
	return &cache
}

func loadSession(path string) (*treestamp.ScanReport, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report treestamp.ScanReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

func saveSession(path string, report *treestamp.ScanReport) error {
	raw, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}
