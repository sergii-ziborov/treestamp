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
	Rows           []Row
	Notes          []string
}

func WriteCard(w io.Writer, pal Palette, card Card) {
	tone := pal.Green
	switch card.Tone {
	case "warn":
		tone = pal.Yellow
	case "bad":
		tone = pal.Red
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n", pal.Paint(tone+pal.Bold, card.Status))
	if card.Detail != "" {
		fmt.Fprintf(w, "  %s\n", pal.Paint(pal.Dim, card.Detail))
	}
	fmt.Fprintln(w)
	width := 0
	for _, row := range card.Rows {
		if n := len(row.Key); n > width {
			width = n
		}
	}
	for _, row := range card.Rows {
		fmt.Fprintf(w, "  %s  %s\n", pal.Paint(pal.Cyan, pad(row.Key, width)), row.Value)
	}
	if len(card.Notes) > 0 {
		fmt.Fprintln(w)
		for _, note := range card.Notes {
			fmt.Fprintf(w, "  %s\n", pal.Paint(pal.Dim, note))
		}
	}
	fmt.Fprintln(w)
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
