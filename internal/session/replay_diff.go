package session

import (
	"fmt"
	"io"
	"sort"
	"time"
)

// DefaultTimeTolerance is the maximum offset difference for two events to be
// considered a potential match during diff alignment.
const DefaultTimeTolerance = 2 * time.Second

// DiffStatus classifies a paired event in a diff result.
type DiffStatus string

const (
	DiffMatched  DiffStatus = "matched"  // present in both, same data
	DiffModified DiffStatus = "modified" // present in both, different data
	DiffOnlyA    DiffStatus = "only_a"   // present in A only
	DiffOnlyB    DiffStatus = "only_b"   // present in B only
)

// DiffEvent pairs events from two replay sessions.
type DiffEvent struct {
	Status   DiffStatus   `json:"status"`
	EventA   *ReplayEvent `json:"event_a,omitempty"`
	EventB   *ReplayEvent `json:"event_b,omitempty"`
	OffsetA  time.Duration `json:"offset_a,omitempty"`
	OffsetB  time.Duration `json:"offset_b,omitempty"`
}

// DiffResult summarises the comparison of two session replays.
type DiffResult struct {
	SessionIDA string        `json:"session_id_a"`
	SessionIDB string        `json:"session_id_b"`
	TotalA     int           `json:"total_a"`
	TotalB     int           `json:"total_b"`
	Matched    int           `json:"matched"`
	Modified   int           `json:"modified"`
	OnlyA      int           `json:"only_a"`
	OnlyB      int           `json:"only_b"`
	Similarity float64       `json:"similarity"` // 0.0–1.0
	DurationA  time.Duration `json:"duration_a"`
	DurationB  time.Duration `json:"duration_b"`
	Events     []DiffEvent   `json:"events"`
}

// NewPlayerFromEvents creates a Player from a pre-built slice of events.
// This is primarily useful for testing.
func NewPlayerFromEvents(events []ReplayEvent) *Player {
	cp := make([]ReplayEvent, len(events))
	copy(cp, events)
	return &Player{events: cp}
}

// DiffSessions compares two session replays using timestamp-offset alignment.
// Events are aligned by their offset from the first event in each session;
// two events are candidates for matching when they share the same type and
// their offset difference is within tolerance.
func DiffSessions(a, b *Player) (*DiffResult, error) {
	return DiffSessionsWithTolerance(a, b, DefaultTimeTolerance)
}

// DiffSessionsWithTolerance is like DiffSessions but accepts a custom time tolerance.
func DiffSessionsWithTolerance(a, b *Player, tolerance time.Duration) (*DiffResult, error) {
	if a == nil || b == nil {
		return nil, fmt.Errorf("replay diff: both players must be non-nil")
	}

	evA := a.Events()
	evB := b.Events()

	result := &DiffResult{
		TotalA:    len(evA),
		TotalB:    len(evB),
		DurationA: a.Duration(),
		DurationB: b.Duration(),
	}

	// Identify session IDs from first events.
	if len(evA) > 0 {
		result.SessionIDA = evA[0].SessionID
	}
	if len(evB) > 0 {
		result.SessionIDB = evB[0].SessionID
	}

	// Empty cases.
	if len(evA) == 0 && len(evB) == 0 {
		result.Similarity = 1.0
		return result, nil
	}
	if len(evA) == 0 {
		for i := range evB {
			result.Events = append(result.Events, DiffEvent{
				Status:  DiffOnlyB,
				EventB:  &evB[i],
				OffsetB: offsetFrom(evB, i),
			})
		}
		result.OnlyB = len(evB)
		result.Similarity = 0.0
		return result, nil
	}
	if len(evB) == 0 {
		for i := range evA {
			result.Events = append(result.Events, DiffEvent{
				Status:  DiffOnlyA,
				EventA:  &evA[i],
				OffsetA: offsetFrom(evA, i),
			})
		}
		result.OnlyA = len(evA)
		result.Similarity = 0.0
		return result, nil
	}

	// Greedy forward matching: walk both event lists ordered by offset.
	// For each event in A, try to find the best unmatched event in B that
	// has the same type and is within tolerance of the same offset.
	matchedB := make([]bool, len(evB))
	matchedA := make([]bool, len(evA))
	type matchPair struct {
		ai, bi int
	}
	var pairs []matchPair

	for ai := range evA {
		offA := offsetFrom(evA, ai)
		bestBI := -1
		bestDelta := tolerance + 1

		for bi := range evB {
			if matchedB[bi] {
				continue
			}
			if evB[bi].Type != evA[ai].Type {
				continue
			}
			offB := offsetFrom(evB, bi)
			delta := absDuration(offA - offB)
			if delta <= tolerance && delta < bestDelta {
				bestDelta = delta
				bestBI = bi
			}
		}

		if bestBI >= 0 {
			matchedA[ai] = true
			matchedB[bestBI] = true
			pairs = append(pairs, matchPair{ai, bestBI})
		}
	}

	// Build diff events: first paired, then only-A, then only-B.
	// Sort pairs by offset so output is chronological.
	sort.Slice(pairs, func(i, j int) bool {
		return offsetFrom(evA, pairs[i].ai) < offsetFrom(evA, pairs[j].ai)
	})

	for _, p := range pairs {
		de := DiffEvent{
			EventA:  &evA[p.ai],
			EventB:  &evB[p.bi],
			OffsetA: offsetFrom(evA, p.ai),
			OffsetB: offsetFrom(evB, p.bi),
		}
		if evA[p.ai].Data == evB[p.bi].Data {
			de.Status = DiffMatched
			result.Matched++
		} else {
			de.Status = DiffModified
			result.Modified++
		}
		result.Events = append(result.Events, de)
	}

	for ai := range evA {
		if !matchedA[ai] {
			result.Events = append(result.Events, DiffEvent{
				Status:  DiffOnlyA,
				EventA:  &evA[ai],
				OffsetA: offsetFrom(evA, ai),
			})
			result.OnlyA++
		}
	}

	for bi := range evB {
		if !matchedB[bi] {
			result.Events = append(result.Events, DiffEvent{
				Status:  DiffOnlyB,
				EventB:  &evB[bi],
				OffsetB: offsetFrom(evB, bi),
			})
			result.OnlyB++
		}
	}

	// Similarity: matched events / max(totalA, totalB).
	maxTotal := diffMax(len(evA), len(evB))
	if maxTotal > 0 {
		result.Similarity = float64(result.Matched) / float64(maxTotal)
	}

	return result, nil
}

// FormatDiffMarkdown renders a DiffResult as a markdown report.
func FormatDiffMarkdown(diff *DiffResult, w io.Writer) error {
	p := func(format string, args ...any) error {
		_, err := fmt.Fprintf(w, format, args...)
		return err
	}

	if err := p("# Session Replay Diff\n\n"); err != nil {
		return err
	}

	if err := p("## Summary\n\n"); err != nil {
		return err
	}
	if err := p("| Metric | Value |\n|--------|-------|\n"); err != nil {
		return err
	}
	if err := p("| Session A | `%s` |\n", diff.SessionIDA); err != nil {
		return err
	}
	if err := p("| Session B | `%s` |\n", diff.SessionIDB); err != nil {
		return err
	}
	if err := p("| Events in A | %d |\n", diff.TotalA); err != nil {
		return err
	}
	if err := p("| Events in B | %d |\n", diff.TotalB); err != nil {
		return err
	}
	if err := p("| Matched | %d |\n", diff.Matched); err != nil {
		return err
	}
	if err := p("| Modified | %d |\n", diff.Modified); err != nil {
		return err
	}
	if err := p("| Only in A | %d |\n", diff.OnlyA); err != nil {
		return err
	}
	if err := p("| Only in B | %d |\n", diff.OnlyB); err != nil {
		return err
	}
	if err := p("| Similarity | %.1f%% |\n", diff.Similarity*100); err != nil {
		return err
	}
	if err := p("| Duration A | %s |\n", diff.DurationA); err != nil {
		return err
	}
	if err := p("| Duration B | %s |\n\n", diff.DurationB); err != nil {
		return err
	}

	if len(diff.Events) == 0 {
		return p("No events to compare.\n")
	}

	if err := p("## Events\n\n"); err != nil {
		return err
	}

	for i, de := range diff.Events {
		switch de.Status {
		case DiffMatched:
			if err := p("### %d. MATCHED `%s` @ %s\n\n", i+1, de.EventA.Type, de.OffsetA); err != nil {
				return err
			}
			if err := p("```\n%s\n```\n\n", diffTruncate(de.EventA.Data, 200)); err != nil {
				return err
			}
		case DiffModified:
			if err := p("### %d. MODIFIED `%s` @ A:%s / B:%s\n\n", i+1, de.EventA.Type, de.OffsetA, de.OffsetB); err != nil {
				return err
			}
			if err := p("**A:**\n```\n%s\n```\n\n", diffTruncate(de.EventA.Data, 200)); err != nil {
				return err
			}
			if err := p("**B:**\n```\n%s\n```\n\n", diffTruncate(de.EventB.Data, 200)); err != nil {
				return err
			}
		case DiffOnlyA:
			if err := p("### %d. ONLY IN A `%s` @ %s\n\n", i+1, de.EventA.Type, de.OffsetA); err != nil {
				return err
			}
			if err := p("```\n%s\n```\n\n", diffTruncate(de.EventA.Data, 200)); err != nil {
				return err
			}
		case DiffOnlyB:
			if err := p("### %d. ONLY IN B `%s` @ %s\n\n", i+1, de.EventB.Type, de.OffsetB); err != nil {
				return err
			}
			if err := p("```\n%s\n```\n\n", diffTruncate(de.EventB.Data, 200)); err != nil {
				return err
			}
		}
	}

	return nil
}

// offsetFrom returns the duration from the first event in the slice to event[i].
func offsetFrom(events []ReplayEvent, i int) time.Duration {
	if i == 0 || len(events) == 0 {
		return 0
	}
	return events[i].Timestamp.Sub(events[0].Timestamp)
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func diffTruncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func diffMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
