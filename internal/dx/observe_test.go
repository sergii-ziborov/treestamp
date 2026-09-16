package dx

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestSafeCauseHidesAbsolutePath(t *testing.T) {
	err := &fs.PathError{Op: "open", Path: `C:\secret\config.go`, Err: fs.ErrPermission}
	got := SafeCause(err)
	if strings.Contains(got, `C:\secret`) || strings.Contains(got, "C:") {
		t.Fatalf("%q", got)
	}
	if !strings.Contains(got, "config.go") {
		t.Fatalf("%q", got)
	}
}

func TestEscapeNameDoesNotSplitLines(t *testing.T) {
	got := EscapeName("a\r\nb")
	if strings.Contains(got, "\n") || strings.Contains(got, "\r") {
		t.Fatalf("%q", got)
	}
}

func TestAggregatorBoundsSamples(t *testing.T) {
	a := NewAggregator(2)
	for i := 0; i < 100; i++ {
		a.Add("read", "x")
	}
	if a.Count("read") != 100 || len(a.Samples("read")) != 2 {
		t.Fatalf("%d %v", a.Count("read"), a.Samples("read"))
	}
}

func TestLogEventSilentWhenDisabled(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))
	LogEvent(context.Background(), log, slog.LevelInfo, "scan.finished", "", "", slog.Int("n", 1))
	if buf.Len() != 0 {
		t.Fatalf("%q", buf.String())
	}
}

func TestPaceAndBundle(t *testing.T) {
	var p Pace
	now := time.Now()
	if !p.Allow(now) || p.Allow(now.Add(time.Millisecond)) || !p.Allow(now.Add(ProgressGap)) {
		t.Fatal("pace")
	}
	var buf bytes.Buffer
	if err := WriteBundle(&buf, "treestamp", "profile=repository", "files=1", []string{"partial"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "library=treestamp") || strings.Contains(buf.String(), "C:\\") {
		t.Fatalf("%q", buf.String())
	}
}

func TestJoinSampled(t *testing.T) {
	var errs []error
	for i := 0; i < 20; i++ {
		errs = append(errs, errors.New("x"))
	}
	err := JoinSampled(errs, 3)
	if err == nil || strings.Count(err.Error(), "x") > 4 {
		t.Fatalf("%v", err)
	}
}
