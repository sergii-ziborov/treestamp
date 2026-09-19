// Package watch adapts fsnotify events into Treestamp watch plans.
//
// It is a separate module. The main Treestamp module must not require fsnotify.
package watch

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
	"github.com/sergii-ziborov/treestamp"
)

// ErrClosed is returned after Close, or when native channels shut down
// because the watcher was closed.
var ErrClosed = errors.New("watch: closed")

// Watcher maps native filesystem notifications onto [treestamp.WatchPlan].
type Watcher struct {
	fs      *fsnotify.Watcher
	adapter *treestamp.WatcherEventAdapter
	closed  atomic.Bool
}

// Open starts a recursive watcher on root. Ignore file names mark
// control-file edits as a full rescan.
func Open(root string, ignoreFiles []string) (*Watcher, error) {
	native, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := addTree(native, root); err != nil {
		_ = native.Close()
		return nil, err
	}
	adapter, err := treestamp.NewWatcherEventAdapter(root, ignoreFiles)
	if err != nil {
		_ = native.Close()
		return nil, err
	}
	return &Watcher{fs: native, adapter: adapter}, nil
}

func addTree(native *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return native.Add(path)
		}
		return nil
	})
}

func (w *Watcher) Close() error {
	if w == nil || w.fs == nil {
		return nil
	}
	w.closed.Store(true)
	return w.fs.Close()
}

// Plan waits for the next native event and returns the corresponding plan.
// Uncertain coverage becomes a full rescan. A closed watcher is ErrClosed,
// not a silent full rescan.
func (w *Watcher) Plan(ctx context.Context) (treestamp.WatchPlan, error) {
	if w == nil || w.closed.Load() {
		return treestamp.WatchPlan{}, ErrClosed
	}
	select {
	case <-ctx.Done():
		return treestamp.WatchPlan{}, ctx.Err()
	case err, ok := <-w.fs.Errors:
		if !ok {
			return closedOrLost(w)
		}
		return treestamp.WatchPlan{FullRescan: true, RejectedEvents: 1}, err
	case ev, ok := <-w.fs.Events:
		if !ok {
			return closedOrLost(w)
		}
		watchCreated(w.fs, ev)
		return w.adapter.Plan([]treestamp.WatchEvent{mapEvent(ev)}), nil
	}
}

func watchCreated(native *fsnotify.Watcher, ev fsnotify.Event) {
	if !ev.Has(fsnotify.Create) {
		return
	}
	info, err := os.Stat(ev.Name)
	if err != nil || !info.IsDir() {
		return
	}
	_ = addTree(native, ev.Name)
}

func closedOrLost(w *Watcher) (treestamp.WatchPlan, error) {
	if w.closed.Load() {
		return treestamp.WatchPlan{}, ErrClosed
	}
	return treestamp.WatchPlan{FullRescan: true}, nil
}

func mapEvent(ev fsnotify.Event) treestamp.WatchEvent {
	kind := treestamp.WatchModify
	switch {
	case ev.Has(fsnotify.Create):
		kind = treestamp.WatchCreate
	case ev.Has(fsnotify.Remove):
		kind = treestamp.WatchRemove
	case ev.Has(fsnotify.Rename):
		kind = treestamp.WatchRenameFrom
	}
	return treestamp.NewWatchEvent(ev.Name, kind)
}
