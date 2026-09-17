// Package watch adapts fsnotify events into Treestamp watch plans.
//
// It is a separate module. The main Treestamp module must not require fsnotify.
package watch

import (
	"context"

	"github.com/fsnotify/fsnotify"
	"github.com/sergii-ziborov/treestamp"
)

// Watcher maps native filesystem notifications onto [treestamp.WatchPlan].
type Watcher struct {
	fs      *fsnotify.Watcher
	adapter *treestamp.WatcherEventAdapter
}

// Open starts a watcher on root. Ignore file names mark control-file edits
// as a full rescan.
func Open(root string, ignoreFiles []string) (*Watcher, error) {
	fs, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := fs.Add(root); err != nil {
		_ = fs.Close()
		return nil, err
	}
	adapter, err := treestamp.NewWatcherEventAdapter(root, ignoreFiles)
	if err != nil {
		_ = fs.Close()
		return nil, err
	}
	return &Watcher{fs: fs, adapter: adapter}, nil
}

func (w *Watcher) Close() error {
	if w == nil || w.fs == nil {
		return nil
	}
	return w.fs.Close()
}

// Plan waits for the next native event and returns the corresponding plan.
// Uncertain coverage becomes a full rescan.
func (w *Watcher) Plan(ctx context.Context) (treestamp.WatchPlan, error) {
	select {
	case <-ctx.Done():
		return treestamp.WatchPlan{}, ctx.Err()
	case err, ok := <-w.fs.Errors:
		if !ok {
			return treestamp.WatchPlan{FullRescan: true}, nil
		}
		return treestamp.WatchPlan{FullRescan: true, RejectedEvents: 1}, err
	case ev, ok := <-w.fs.Events:
		if !ok {
			return treestamp.WatchPlan{FullRescan: true}, nil
		}
		return w.adapter.Plan([]treestamp.WatchEvent{mapEvent(ev)}), nil
	}
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
