package dx

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const ProgressGap = 250 * time.Millisecond

type Progress struct {
	Phase                        string
	Discovered, Processed, Bytes uint64
	Elapsed                      time.Duration
}

type LimitMode uint8

const (
	LimitUnset LimitMode = iota
	LimitZero
	LimitValue
	LimitUnlimited
)

type ByteLimit struct {
	Mode LimitMode
	N    uint64
}

func LimitBytes(n uint64) ByteLimit { return ByteLimit{Mode: LimitValue, N: n} }
func ZeroBytes() ByteLimit          { return ByteLimit{Mode: LimitZero} }
func UnlimitedBytes() ByteLimit     { return ByteLimit{Mode: LimitUnlimited} }

type Aggregator struct {
	counts  map[string]int
	samples map[string][]string
	max     int
}

func NewAggregator(maxSamples int) *Aggregator {
	if maxSamples <= 0 {
		maxSamples = 3
	}
	return &Aggregator{counts: map[string]int{}, samples: map[string][]string{}, max: maxSamples}
}

func (a *Aggregator) Add(key, sample string) {
	if a == nil {
		return
	}
	a.counts[key]++
	if len(a.samples[key]) < a.max {
		a.samples[key] = append(a.samples[key], EscapeName(sample))
	}
}

func (a *Aggregator) Count(key string) int {
	if a == nil {
		return 0
	}
	return a.counts[key]
}

func (a *Aggregator) Samples(key string) []string {
	if a == nil {
		return nil
	}
	return append([]string(nil), a.samples[key]...)
}

func EscapeName(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		i += n
		switch r {
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func SafeCause(err error) string {
	if err == nil {
		return ""
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Op + " " + filepath.Base(pe.Path) + ": " + pe.Err.Error()
	}
	var oe *os.PathError
	if errors.As(err, &oe) {
		return oe.Op + " " + filepath.Base(oe.Path) + ": " + oe.Err.Error()
	}
	return EscapeName(err.Error())
}

func LogEvent(ctx context.Context, log *slog.Logger, level slog.Level, event, code, action string, attrs ...slog.Attr) {
	if log == nil || !log.Enabled(ctx, level) {
		return
	}
	out := make([]slog.Attr, 0, len(attrs)+3)
	out = append(out, slog.String("event", event))
	if code != "" {
		out = append(out, slog.String("code", code))
	}
	if action != "" {
		out = append(out, slog.String("action", action))
	}
	out = append(out, attrs...)
	log.LogAttrs(ctx, level, event, out...)
}

type Pace struct{ last time.Time }

func (p *Pace) Allow(now time.Time) bool {
	if p.last.IsZero() || now.Sub(p.last) >= ProgressGap {
		p.last = now
		return true
	}
	return false
}

func WriteBundle(w io.Writer, library, describe, summary string, codes []string) error {
	if w == nil {
		return errors.New("nil writer")
	}
	var b strings.Builder
	b.WriteString("library=" + library + "\n")
	b.WriteString("go=" + runtime.Version() + "\n")
	b.WriteString("os=" + runtime.GOOS + "/" + runtime.GOARCH + "\n")
	b.WriteString("describe=" + describe + "\n")
	if len(codes) > 0 {
		b.WriteString("codes=" + strings.Join(codes, ",") + "\n")
	}
	if summary != "" {
		b.WriteString("summary=" + summary + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func JoinSampled(errs []error, max int) error {
	if len(errs) == 0 {
		return nil
	}
	if max <= 0 {
		max = 8
	}
	if len(errs) == 1 {
		return errs[0]
	}
	n := len(errs)
	if n > max {
		errs = errs[:max]
	}
	return errors.Join(append(errs, errorCount(n))...)
}

type errorCount int

func (e errorCount) Error() string { return "sampled " + strconv.Itoa(int(e)) + " errors" }
