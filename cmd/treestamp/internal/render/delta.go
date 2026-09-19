package render

import (
	"io"

	"github.com/sergii-ziborov/treestamp"
)

type Rename struct {
	From, To string
}

type Delta struct {
	Added, Removed, Changed []string
	Renames                 []Rename
}

func OfDelta(d treestamp.ScanDelta) Delta {
	out := Delta{
		Added:   make([]string, 0, len(d.Added)),
		Removed: make([]string, 0, len(d.Removed)),
		Changed: make([]string, 0, len(d.Modified)),
		Renames: make([]Rename, 0, len(d.Renamed)),
	}
	for _, file := range d.Added {
		out.Added = append(out.Added, file.Relative)
	}
	for _, file := range d.Removed {
		out.Removed = append(out.Removed, file.Relative)
	}
	for _, item := range d.Modified {
		out.Changed = append(out.Changed, item.Current.Relative)
	}
	for _, item := range d.Renamed {
		out.Renames = append(out.Renames, Rename{From: item.Previous.Relative, To: item.Current.Relative})
	}
	return out
}

func (d Delta) Rows() []Row {
	return []Row{
		CountRow("Added", len(d.Added)),
		CountRow("Removed", len(d.Removed)),
		CountRow("Changed", len(d.Changed)),
		CountRow("Renamed", len(d.Renames)),
	}
}

func (d Delta) Paths() []string {
	out := make([]string, 0, len(d.Added)+len(d.Removed)+len(d.Changed)+2*len(d.Renames))
	out = append(out, d.Added...)
	out = append(out, d.Removed...)
	out = append(out, d.Changed...)
	for _, item := range d.Renames {
		out = append(out, item.From, item.To)
	}
	return out
}

func (d Delta) Lines() []string {
	out := make([]string, 0, len(d.Added)+len(d.Removed)+len(d.Changed)+len(d.Renames))
	for _, path := range d.Added {
		out = append(out, "+ "+path)
	}
	for _, path := range d.Removed {
		out = append(out, "- "+path)
	}
	for _, path := range d.Changed {
		out = append(out, "~ "+path)
	}
	for _, item := range d.Renames {
		out = append(out, item.From+" → "+item.To)
	}
	return out
}

func WriteNull(w io.Writer, lines []string) error {
	for _, line := range lines {
		if _, err := io.WriteString(w, line+"\x00"); err != nil {
			return err
		}
	}
	return nil
}
