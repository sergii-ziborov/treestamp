package listwalk

import (
	"errors"
	"io/fs"

	"github.com/sergii-ziborov/treestamp/internal/dirread"
)

const defaultMaxOpen = 64

var testLazyChild func(string)

type state struct {
	root   string
	fn     fs.WalkDirFunc
	cfg    Config
	frames []frame
	skip   string
}

func (st *state) run() error {
	st.frames = []frame{{path: st.root}}
	defer st.closeAll()
	for len(st.frames) > 0 {
		if err := st.stopped(); err != nil {
			return err
		}
		if err := st.step(); err != nil {
			if errors.Is(err, errStop) {
				return nil
			}
			return err
		}
	}
	return nil
}

func (st *state) stopped() error {
	if st.cfg.Context == nil {
		return nil
	}
	return st.cfg.Context.Err()
}

func (st *state) step() error {
	top := &st.frames[len(st.frames)-1]
	if err := st.fill(top); err != nil {
		return err
	}
	rec, ok, err := st.next(top)
	if err != nil {
		return err
	}
	if !ok {
		return finish(st.root, st.fn, st.cfg, top, &st.skip, &st.frames)
	}
	return visit(st.fn, st.cfg, top, rec, &st.frames, &st.skip)
}

func (st *state) fill(top *frame) error {
	if top.ready {
		return nil
	}
	if st.cfg.Sort || st.cfg.ContentsFirst {
		return fill(top, st.fn, st.cfg)
	}
	return st.openLazy(top)
}

func (st *state) openLazy(top *frame) error {
	top.ready = true
	if err := st.bufferIfNeeded(); err != nil {
		return reportRead(st.fn, st.cfg, top.path, err)
	}
	scan, err := dirread.NewScanner(top.path, st.scratchForOpen())
	if err != nil {
		return reportRead(st.fn, st.cfg, top.path, err)
	}
	top.scan = scan
	return nil
}

func (st *state) scratchForOpen() []byte {
	if st.openCount() == 0 {
		return st.cfg.Scratch
	}
	return nil
}

func (st *state) next(top *frame) (dirread.Record, bool, error) {
	if top.done {
		return dirread.Record{}, false, nil
	}
	if top.scan != nil {
		return st.nextScan(top)
	}
	if top.index < len(top.dents) {
		rec := top.dents[top.index]
		top.index++
		return rec, true, nil
	}
	return dirread.Record{}, false, nil
}

func (st *state) nextScan(top *frame) (dirread.Record, bool, error) {
	if !top.scan.Scan() {
		err := top.scan.Err()
		top.scan = nil
		top.done = true
		if err != nil {
			return dirread.Record{}, false, reportRead(st.fn, st.cfg, top.path, err)
		}
		return dirread.Record{}, false, nil
	}
	rec := top.scan.Record()
	if testLazyChild != nil {
		testLazyChild(rec.Name)
	}
	return rec, true, nil
}

func (st *state) maxOpen() int {
	if st.cfg.MaxOpen > 0 {
		return st.cfg.MaxOpen
	}
	return defaultMaxOpen
}

func (st *state) openCount() int {
	n := 0
	for i := range st.frames {
		if st.frames[i].scan != nil {
			n++
		}
	}
	return n
}

func (st *state) bufferIfNeeded() error {
	for st.openCount() >= st.maxOpen() {
		if err := st.bufferOldest(); err != nil {
			return err
		}
	}
	return nil
}

func (st *state) bufferOldest() error {
	for i := range st.frames {
		if st.frames[i].scan == nil {
			continue
		}
		return bufferFrame(&st.frames[i])
	}
	return nil
}

func bufferFrame(fr *frame) error {
	for fr.scan.Scan() {
		fr.dents = append(fr.dents, fr.scan.Record())
	}
	err := fr.scan.Err()
	fr.scan = nil
	return err
}

func (st *state) closeAll() {
	for i := range st.frames {
		if st.frames[i].scan != nil {
			_ = st.frames[i].scan.Close()
			st.frames[i].scan = nil
		}
	}
}

func (f *frame) exhaust() {
	if f.scan != nil {
		_ = f.scan.Close()
		f.scan = nil
	}
	f.index = len(f.dents)
	f.done = true
}
