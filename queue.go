//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tailscale/walk"

	"vidpress/engine"
)

// ItemState is the life cycle of a queue entry.
type ItemState int

const (
	StProbing  ItemState = iota // reading media information
	StReady                     // waiting to be converted
	StRunning                   // being converted
	StDone                      // finished successfully
	StFailed                    // conversion failed
	StCanceled                  // stopped by the user
	StInvalid                   // could not be read at all
)

// Item is one file in the queue.
type Item struct {
	Path      string
	Name      string
	Info      *engine.MediaInfo
	ProbeErr  error
	State     ItemState
	Progress  float64 // 0..1 while running, <0 = unknown
	PassLabel string
	Result    *engine.Result
	Err       error
	Batch     int // batch number that claimed this item
}

// Column indexes.
const (
	colName = iota
	colSize
	colDuration
	colResolution
	colCodec
	colStatus
	colResult
)

// QueueModel is the walk.TableModel behind the file list. Item fields are
// protected by mu because the conversion goroutine updates them.
type QueueModel struct {
	walk.TableModelBase
	mu    sync.Mutex
	items []*Item
}

func (m *QueueModel) RowCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}

func (m *QueueModel) Value(row, col int) interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	if row < 0 || row >= len(m.items) {
		return ""
	}
	it := m.items[row]
	switch col {
	case colName:
		return it.Name
	case colSize:
		if it.Info != nil {
			return engine.FormatSize(it.Info.Size)
		}
		return ""
	case colDuration:
		if it.Info != nil {
			return engine.FormatDuration(it.Info.Duration)
		}
		return ""
	case colResolution:
		if it.Info != nil {
			return it.Info.ResolutionText()
		}
		return ""
	case colCodec:
		if it.Info != nil {
			return it.Info.CodecText()
		}
		return ""
	case colStatus:
		return statusText(it)
	case colResult:
		switch it.State {
		case StDone:
			if it.Result != nil && it.Info != nil {
				return engine.SavingText(it.Info.Size, it.Result.Size)
			}
		case StFailed:
			if it.Err != nil {
				return it.Err.Error()
			}
		case StInvalid:
			if it.ProbeErr != nil {
				return it.ProbeErr.Error()
			}
		}
		return ""
	}
	return ""
}

func statusText(it *Item) string {
	switch it.State {
	case StProbing:
		return "정보 읽는 중…"
	case StReady:
		return "대기"
	case StRunning:
		if it.Progress < 0 {
			return it.PassLabel
		}
		return fmt.Sprintf("%s %d%%", it.PassLabel, int(it.Progress*100))
	case StDone:
		return "완료"
	case StFailed:
		return "실패"
	case StCanceled:
		return "중지됨"
	case StInvalid:
		return "읽을 수 없음"
	}
	return ""
}

// Add appends paths that are not in the queue yet and returns the new items.
func (m *QueueModel) Add(paths []string) []*Item {
	m.mu.Lock()
	existing := make(map[string]bool, len(m.items))
	for _, it := range m.items {
		existing[strings.ToLower(it.Path)] = true
	}
	var added []*Item
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err == nil {
			p = abs
		}
		key := strings.ToLower(p)
		if existing[key] {
			continue
		}
		existing[key] = true
		it := &Item{Path: p, Name: filepath.Base(p), State: StProbing}
		m.items = append(m.items, it)
		added = append(added, it)
	}
	from, to := len(m.items)-len(added), len(m.items)-1
	m.mu.Unlock()
	if len(added) > 0 {
		m.PublishRowsInserted(from, to)
	}
	return added
}

// IndexOf returns the row of it, or -1.
func (m *QueueModel) IndexOf(it *Item) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, x := range m.items {
		if x == it {
			return i
		}
	}
	return -1
}

// NotifyChanged publishes a change of it (UI thread only).
func (m *QueueModel) NotifyChanged(it *Item) {
	if i := m.IndexOf(it); i >= 0 {
		m.PublishRowChanged(i)
	}
}

// Remove deletes the given rows (UI thread only).
func (m *QueueModel) Remove(rows []int) {
	if len(rows) == 0 {
		return
	}
	del := make(map[int]bool, len(rows))
	for _, r := range rows {
		del[r] = true
	}
	m.mu.Lock()
	kept := m.items[:0]
	for i, it := range m.items {
		if !del[i] || it.State == StRunning {
			kept = append(kept, it)
		}
	}
	for i := len(kept); i < len(m.items); i++ {
		m.items[i] = nil
	}
	m.items = kept
	m.mu.Unlock()
	m.PublishRowsReset()
}

// Clear removes every item that is not running.
func (m *QueueModel) Clear() {
	m.mu.Lock()
	kept := m.items[:0]
	for _, it := range m.items {
		if it.State == StRunning {
			kept = append(kept, it)
		}
	}
	m.items = kept
	m.mu.Unlock()
	m.PublishRowsReset()
}

// Item returns the item at row (or nil).
func (m *QueueModel) Item(row int) *Item {
	m.mu.Lock()
	defer m.mu.Unlock()
	if row < 0 || row >= len(m.items) {
		return nil
	}
	return m.items[row]
}

// Snapshot returns a copy of the item list.
func (m *QueueModel) Snapshot() []*Item {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*Item(nil), m.items...)
}

// Update runs fn with the lock held.
func (m *QueueModel) Update(it *Item, fn func(it *Item)) {
	m.mu.Lock()
	fn(it)
	m.mu.Unlock()
}

// ClaimNext marks the next ready item as running for batch and returns it.
func (m *QueueModel) ClaimNext(batch int) *Item {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, it := range m.items {
		if it.State == StReady {
			it.State = StRunning
			it.Progress = 0
			it.PassLabel = "준비 중"
			it.Err = nil
			it.Batch = batch
			return it
		}
	}
	return nil
}

// Stats summarises the queue.
type Stats struct {
	Total, Probing, Ready, Running, Done, Failed, Canceled, Invalid int
	// Batch progress
	BatchTotal, BatchFinished int
	RunningFraction           float64
	InSize, OutSize           int64 // sizes of files finished in the batch
	TotalSize                 int64 // total size of readable files
}

func (m *QueueModel) Stats(batch int) Stats {
	m.mu.Lock()
	defer m.mu.Unlock()
	var s Stats
	for _, it := range m.items {
		s.Total++
		if it.Info != nil {
			s.TotalSize += it.Info.Size
		}
		inBatch := batch > 0 && it.Batch == batch
		switch it.State {
		case StProbing:
			s.Probing++
		case StReady:
			s.Ready++
			if batch > 0 {
				s.BatchTotal++
			}
		case StRunning:
			s.Running++
			if inBatch {
				s.BatchTotal++
				if it.Progress > 0 {
					s.RunningFraction = it.Progress
				}
			}
		case StDone:
			s.Done++
			if inBatch {
				s.BatchTotal++
				s.BatchFinished++
				if it.Info != nil && it.Result != nil {
					s.InSize += it.Info.Size
					s.OutSize += it.Result.Size
				}
			}
		case StFailed:
			s.Failed++
			if inBatch {
				s.BatchTotal++
				s.BatchFinished++
			}
		case StCanceled:
			s.Canceled++
		case StInvalid:
			s.Invalid++
		}
	}
	return s
}

// Compile-time check that QueueModel satisfies walk.TableModel.
var _ walk.TableModel = (*QueueModel)(nil)
