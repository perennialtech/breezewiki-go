package archive

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

type QueueItem struct {
	Wiki     string `json:"wiki"`
	State    string `json:"state"`
	Stage    int    `json:"stage"`
	Progress int    `json:"progress"`
	Total    int    `json:"total"`
	Name     string `json:"name"`
	Error    string `json:"error,omitempty"`
}

type queueEntry struct {
	QueueItem
	cancel context.CancelFunc
	done   chan struct{}
}

// Queue coordinates the GUI's download, pause, resume, reset, and reset-pages
// actions with durable per-wiki progress in SQLite.
type Queue struct {
	archive *Archive
	mu      sync.Mutex
	items   []*queueEntry
	closed  bool
}

func NewQueue(a *Archive) (*Queue, error) {
	pending, err := a.Pending()
	if err != nil {
		return nil, err
	}
	q := &Queue{archive: a}
	for wiki, progress := range pending {
		q.items = append(q.items, &queueEntry{QueueItem: QueueItem{Wiki: wiki, State: "queued", Stage: progress, Total: 1}})
	}
	sort.Slice(q.items, func(i, j int) bool { return q.items[i].Wiki < q.items[j].Wiki })
	return q, nil
}

func (q *Queue) find(wiki string) *queueEntry {
	for _, entry := range q.items {
		if entry.Wiki == wiki {
			return entry
		}
	}
	return nil
}

func (q *Queue) Add(wiki string) error {
	if !ValidWiki(wiki) {
		return fmt.Errorf("invalid Fandom wiki name %q", wiki)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errors.New("queue closed")
	}
	if q.find(wiki) == nil {
		q.items = append(q.items, &queueEntry{QueueItem: QueueItem{Wiki: wiki, State: "queued", Total: 1}})
	}
	return nil
}

func (q *Queue) Snapshot() []QueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()
	copy := make([]QueueItem, 0, len(q.items))
	for _, entry := range q.items {
		copy = append(copy, entry.QueueItem)
	}
	return copy
}

func (q *Queue) Resume(wiki string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errors.New("queue closed")
	}
	entry := q.find(wiki)
	if entry == nil {
		return fmt.Errorf("wiki %q is not queued", wiki)
	}
	if entry.State == "running" {
		return nil
	}
	q.startLocked(entry)
	return nil
}

func (q *Queue) startLocked(entry *queueEntry) {
	ctx, cancel := context.WithCancel(context.Background())
	entry.cancel, entry.done = cancel, make(chan struct{})
	entry.State, entry.Error, entry.Progress, entry.Total = "running", "", 0, 1
	go func() {
		err := q.archive.Run(ctx, entry.Wiki, func(stage, done, total int, name string) {
			q.mu.Lock()
			if entry.State == "running" {
				entry.Stage, entry.Progress, entry.Total, entry.Name = stage, done, total, name
			}
			q.mu.Unlock()
		})
		q.mu.Lock()
		advance := entry.State == "running"
		if advance {
			if err == nil {
				entry.State, entry.Stage, entry.Progress = "complete", len(StageNames), entry.Total
			} else if errors.Is(err, context.Canceled) {
				entry.State = "paused"
			} else {
				entry.State, entry.Error = "error", err.Error()
			}
		}
		close(entry.done)
		entry.cancel = nil
		entry.done = nil
		// The original GUI advances to the last queued or paused item
		// after completion/error, but not after the user clicks Pause.
		if !q.closed && advance {
			for i := len(q.items) - 1; i >= 0; i-- {
				next := q.items[i]
				if next.State == "queued" || next.State == "paused" {
					q.startLocked(next)
					break
				}
			}
		}
		q.mu.Unlock()
	}()
}

func (q *Queue) Pause(wiki string) error {
	q.mu.Lock()
	entry := q.find(wiki)
	if entry == nil {
		q.mu.Unlock()
		return fmt.Errorf("wiki %q is not queued", wiki)
	}
	if entry.State != "running" {
		q.mu.Unlock()
		return nil
	}
	entry.State = "paused"
	entry.cancel()
	done := entry.done
	q.mu.Unlock()
	<-done
	return nil
}

func (q *Queue) Reset(wiki string, deletePages bool) error {
	if err := q.Pause(wiki); err != nil {
		return err
	}
	if err := q.archive.Reset(wiki, deletePages); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	entry := q.find(wiki)
	if entry == nil {
		return fmt.Errorf("wiki %q is not queued", wiki)
	}
	entry.State, entry.Stage, entry.Progress, entry.Total, entry.Error, entry.Name = "queued", 0, 0, 1, "", ""
	return nil
}

func (q *Queue) Close() {
	q.mu.Lock()
	q.closed = true
	var cancels []context.CancelFunc
	var dones []chan struct{}
	for _, entry := range q.items {
		if entry.cancel != nil {
			cancels = append(cancels, entry.cancel)
			dones = append(dones, entry.done)
		}
	}
	q.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	for _, done := range dones {
		<-done
	}
}
