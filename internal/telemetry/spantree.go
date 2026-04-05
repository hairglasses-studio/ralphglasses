package telemetry

import (
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SpanStatus represents the completion state of a span.
type SpanStatus string

const (
	SpanStatusRunning SpanStatus = "running"
	SpanStatusOK      SpanStatus = "ok"
	SpanStatusError   SpanStatus = "error"
)

// Span is a single unit of work within a SpanTree.
type Span struct {
	ID         string            `json:"id"`
	ParentID   string            `json:"parent_id,omitempty"`
	Name       string            `json:"name"`
	Status     SpanStatus        `json:"status"`
	StartTime  time.Time         `json:"start_time"`
	EndTime    time.Time         `json:"end_time"`
	Duration   time.Duration     `json:"duration,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`

	children []*Span // populated by buildChildren during render
}

// spanJSON is the JSON serialization form with duration as a string.
type spanJSON struct {
	ID         string            `json:"id"`
	ParentID   string            `json:"parent_id,omitempty"`
	Name       string            `json:"name"`
	Status     SpanStatus        `json:"status"`
	StartTime  time.Time         `json:"start_time"`
	EndTime    time.Time         `json:"end_time"`
	Duration   string            `json:"duration,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Children   []*spanJSON       `json:"children,omitempty"`
}

// SpanTree collects spans and supports rendering them as an ASCII tree,
// JSON object, or flat list sorted by start time. All methods are safe
// for concurrent use.
type SpanTree struct {
	mu    sync.RWMutex
	spans map[string]*Span
	seq   atomic.Int64
}

// NewSpanTree creates an empty SpanTree.
func NewSpanTree() *SpanTree {
	return &SpanTree{
		spans: make(map[string]*Span),
	}
}

// StartSpan begins a new span under the given parent (use "" for a root span).
// It returns the span ID which must be passed to EndSpan when the work completes.
func (t *SpanTree) StartSpan(parentID, name string) string {
	id := fmt.Sprintf("span-%d", t.seq.Add(1))

	span := &Span{
		ID:         id,
		ParentID:   parentID,
		Name:       name,
		Status:     SpanStatusRunning,
		StartTime:  time.Now(),
		Attributes: make(map[string]string),
	}

	t.mu.Lock()
	t.spans[id] = span
	t.mu.Unlock()

	return id
}

// EndSpan marks a span as completed with the given status. If status is empty,
// SpanStatusOK is used. Returns false if the span ID was not found.
func (t *SpanTree) EndSpan(id string, status SpanStatus) bool {
	if status == "" {
		status = SpanStatusOK
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	span, ok := t.spans[id]
	if !ok {
		return false
	}
	now := time.Now()
	span.EndTime = now
	span.Duration = now.Sub(span.StartTime)
	span.Status = status
	return true
}

// SetAttribute sets a key-value attribute on the given span.
// Returns false if the span ID was not found.
func (t *SpanTree) SetAttribute(id, key, value string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	span, ok := t.spans[id]
	if !ok {
		return false
	}
	span.Attributes[key] = value
	return true
}

// Span returns a copy of the span with the given ID, or nil if not found.
func (t *SpanTree) Span(id string) *Span {
	t.mu.RLock()
	defer t.mu.RUnlock()

	span, ok := t.spans[id]
	if !ok {
		return nil
	}
	cp := *span
	cp.Attributes = make(map[string]string, len(span.Attributes))
	maps.Copy(cp.Attributes, span.Attributes)
	return &cp
}

// Len returns the number of spans in the tree.
func (t *SpanTree) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.spans)
}

// FlatList returns all spans sorted by start time (earliest first).
func (t *SpanTree) FlatList() []*Span {
	t.mu.RLock()
	defer t.mu.RUnlock()

	list := make([]*Span, 0, len(t.spans))
	for _, s := range t.spans {
		cp := *s
		cp.Attributes = make(map[string]string, len(s.Attributes))
		maps.Copy(cp.Attributes, s.Attributes)
		list = append(list, &cp)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].StartTime.Before(list[j].StartTime)
	})
	return list
}

// buildChildren constructs the parent-child links and returns root spans.
func (t *SpanTree) buildChildren() []*Span {
	// snapshot under read lock
	t.mu.RLock()
	all := make(map[string]*Span, len(t.spans))
	for id, s := range t.spans {
		cp := *s
		cp.Attributes = make(map[string]string, len(s.Attributes))
		maps.Copy(cp.Attributes, s.Attributes)
		cp.children = nil
		all[id] = &cp
	}
	t.mu.RUnlock()

	var roots []*Span
	for _, s := range all {
		if s.ParentID == "" {
			roots = append(roots, s)
			continue
		}
		if parent, ok := all[s.ParentID]; ok {
			parent.children = append(parent.children, s)
		} else {
			// orphan: treat as root
			roots = append(roots, s)
		}
	}

	// sort children by start time at every level
	var sortChildren func(spans []*Span)
	sortChildren = func(spans []*Span) {
		sort.Slice(spans, func(i, j int) bool {
			return spans[i].StartTime.Before(spans[j].StartTime)
		})
		for _, s := range spans {
			sortChildren(s.children)
		}
	}
	sortChildren(roots)

	return roots
}

// RenderTree returns an ASCII tree representation of all spans.
// Each line shows the span name, duration (if ended), and status.
func (t *SpanTree) RenderTree() string {
	roots := t.buildChildren()
	if len(roots) == 0 {
		return "(empty)"
	}

	var b strings.Builder
	for i, root := range roots {
		renderNode(&b, root, "", i == len(roots)-1)
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderNode writes a single span and its children to the builder.
func renderNode(b *strings.Builder, s *Span, prefix string, last bool) {
	connector := "+-"
	if last {
		connector = "`-"
	}
	// root nodes get no connector
	if prefix == "" {
		connector = ""
	}

	label := s.Name
	if s.Duration > 0 {
		label += fmt.Sprintf(" (%s)", s.Duration.Truncate(time.Microsecond))
	}
	label += fmt.Sprintf(" [%s]", s.Status)

	fmt.Fprintf(b, "%s%s %s\n", prefix, connector, label)

	childPrefix := prefix
	if prefix != "" {
		if last {
			childPrefix += "  "
		} else {
			childPrefix += "| "
		}
	}

	for i, child := range s.children {
		renderNode(b, child, childPrefix, i == len(s.children)-1)
	}
}

// RenderJSON returns the span tree as a JSON byte slice with nested children.
func (t *SpanTree) RenderJSON() ([]byte, error) {
	roots := t.buildChildren()
	jsonRoots := make([]*spanJSON, 0, len(roots))
	for _, r := range roots {
		jsonRoots = append(jsonRoots, toSpanJSON(r))
	}
	return json.MarshalIndent(jsonRoots, "", "  ")
}

// toSpanJSON converts a Span (with children) to the JSON serialization form.
func toSpanJSON(s *Span) *spanJSON {
	j := &spanJSON{
		ID:         s.ID,
		ParentID:   s.ParentID,
		Name:       s.Name,
		Status:     s.Status,
		StartTime:  s.StartTime,
		EndTime:    s.EndTime,
		Attributes: s.Attributes,
	}
	if s.Duration > 0 {
		j.Duration = s.Duration.Truncate(time.Microsecond).String()
	}
	if len(s.children) > 0 {
		j.Children = make([]*spanJSON, 0, len(s.children))
		for _, child := range s.children {
			j.Children = append(j.Children, toSpanJSON(child))
		}
	}
	return j
}
