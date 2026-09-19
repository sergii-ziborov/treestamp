package render

import (
	"fmt"
	"io"
	"strings"
)

type Row struct {
	Key, Value string
}

type Card struct {
	Status, Detail string
	Tone           string
	Lead           []string
	Rows           []Row
	Items          []string
	Next           []string
}

type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) Write(p []byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	n, err := e.w.Write(p)
	e.err = err
	return n, err
}

func WriteCard(w io.Writer, pal Palette, card Card) error {
	ew := &errWriter{w: w}
	tone := pal.Green
	switch card.Tone {
	case "warn":
		tone = pal.Yellow
	case "bad":
		tone = pal.Red
	}
	fmt.Fprintf(ew, "%s\n", pal.Paint(tone+pal.Bold, card.Status))
	if card.Detail != "" {
		fmt.Fprintf(ew, "%s\n", pal.Paint(pal.Dim, card.Detail))
	}
	lead, rows := nonempty(card.Lead), filledRows(card.Rows)
	if len(lead) > 0 || len(rows) > 0 {
		fmt.Fprintln(ew)
	}
	writeItems(ew, lead)
	if len(lead) > 0 && len(rows) > 0 {
		fmt.Fprintln(ew)
	}
	width := 0
	for _, row := range rows {
		if n := len(row.Key); n > width {
			width = n
		}
	}
	for _, row := range rows {
		fmt.Fprintf(ew, "%s  %s\n", pal.Paint(pal.Cyan, pad(row.Key, width)), row.Value)
	}
	writeItems(ew, nonempty(card.Items))
	if len(card.Next) > 0 {
		fmt.Fprintln(ew)
		for _, line := range card.Next {
			fmt.Fprintf(ew, "%s\n", pal.Paint(pal.Dim, line))
		}
	}
	return ew.err
}

func writeItems(ew *errWriter, items []string) {
	for _, item := range items {
		fmt.Fprintf(ew, "  %s\n", item)
	}
}

func nonempty(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func filledRows(rows []Row) []Row {
	out := make([]Row, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Key) == "" || strings.TrimSpace(row.Value) == "" {
			continue
		}
		out = append(out, row)
	}
	return out
}

func CountRow(key string, n int) Row {
	if n <= 0 {
		return Row{}
	}
	return Row{Key: key, Value: Comma(n)}
}

func Preview(names []string, limit int) []string {
	if limit <= 0 || len(names) == 0 {
		return nil
	}
	if len(names) <= limit {
		return append([]string(nil), names...)
	}
	shown := append([]string(nil), names[:limit]...)
	return append(shown, fmt.Sprintf("… %d more", len(names)-limit))
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func Comma(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	s := fmt.Sprintf("%d", n)
	var b strings.Builder
	pre := len(s) % 3
	if pre == 0 {
		pre = 3
	}
	b.WriteString(s[:pre])
	for i := pre; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
