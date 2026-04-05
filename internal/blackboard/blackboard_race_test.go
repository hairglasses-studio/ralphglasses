package blackboard

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestBlackboardConcurrentPutGet(t *testing.T) {
	bb := NewBlackboard(t.TempDir())

	var wg sync.WaitGroup

	// 10 writer goroutines each Put 10 entries.
	for w := range 10 {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for k := range 10 {
				err := bb.Put(Entry{
					Key:       fmt.Sprintf("key-%d", k),
					Namespace: fmt.Sprintf("writer-%d", writerID),
					Value:     map[string]any{"n": k},
					WriterID:  fmt.Sprintf("w%d", writerID),
				})
				if err != nil {
					t.Errorf("Put failed: %v", err)
				}
			}
		}(w)
	}

	// 10 reader goroutines each Query repeatedly.
	for r := range 10 {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for range 50 {
				ns := fmt.Sprintf("writer-%d", readerID)
				_ = bb.Query(ns)
				_, _ = bb.Get(ns, "key-0")
			}
		}(r)
	}

	wg.Wait()

	// Verify all entries were written.
	total := bb.Len()
	if total != 100 {
		t.Errorf("expected 100 entries, got %d", total)
	}
}

func TestBlackboardConcurrentWatch(t *testing.T) {
	bb := NewBlackboard(t.TempDir())

	var notified atomic.Int64

	// Register watchers before starting writers.
	for range 3 {
		bb.Watch(func(entry Entry) {
			notified.Add(1)
			// Access entry fields to trigger race detector if data is shared unsafely.
			_ = entry.Key
			_ = entry.Namespace
			_ = entry.Value
		})
	}

	var wg sync.WaitGroup

	numWriters := 10
	putsPerWriter := 10

	for w := range numWriters {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for k := range putsPerWriter {
				err := bb.Put(Entry{
					Key:       fmt.Sprintf("key-%d", k),
					Namespace: fmt.Sprintf("ns-%d", writerID),
					Value:     map[string]any{"v": k},
					WriterID:  fmt.Sprintf("w%d", writerID),
				})
				if err != nil {
					t.Errorf("Put failed: %v", err)
				}
			}
		}(w)
	}

	wg.Wait()

	totalPuts := int64(numWriters * putsPerWriter)
	totalNotifications := notified.Load()
	// 3 watchers * 100 puts = 300 notifications expected.
	expected := totalPuts * 3
	if totalNotifications != expected {
		t.Errorf("expected %d notifications, got %d", expected, totalNotifications)
	}
}
