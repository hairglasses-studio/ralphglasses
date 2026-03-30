package tracing

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// StatusCode represents the span status.
type StatusCode int

const (
	StatusUnset StatusCode = 0
	StatusOK    StatusCode = 1
	StatusError StatusCode = 2
)

// spanContextKey is used to propagate Span through context.Context.
type spanContextKey struct{}

// Span represents a single unit of work in a trace.
type Span struct {
	mu         sync.Mutex
	traceID    string
	spanID     string
	parentID   string
	name       string
	startTime  time.Time
	endTime    time.Time
	attributes map[string]any
	events     []spanEvent
	statusCode StatusCode
	statusMsg  string
	ended      bool
	tracer     *Tracer
}

type spanEvent struct {
	Name       string         `json:"name"`
	Timestamp  time.Time      `json:"timeUnixNano"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// End finishes the span and queues it for export.
func (s *Span) End() {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	s.ended = true
	s.endTime = time.Now()
	s.mu.Unlock()

	if s.tracer != nil {
		s.tracer.enqueue(s)
	}
}

// SetAttribute sets a key-value attribute on the span.
func (s *Span) SetAttribute(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return
	}
	s.attributes[key] = value
}

// SetStatus sets the status code and message on the span.
func (s *Span) SetStatus(code StatusCode, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return
	}
	s.statusCode = code
	s.statusMsg = msg
}

// AddEvent adds a named event with optional attributes to the span.
func (s *Span) AddEvent(name string, attrs ...map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return
	}
	ev := spanEvent{Name: name, Timestamp: time.Now()}
	if len(attrs) > 0 {
		ev.Attributes = attrs[0]
	}
	s.events = append(s.events, ev)
}

// TraceID returns the span's trace ID.
func (s *Span) TraceID() string {
	return s.traceID
}

// SpanID returns the span's span ID.
func (s *Span) SpanID() string {
	return s.spanID
}

// TracerOption configures a Tracer.
type TracerOption func(*tracerConfig)

type tracerConfig struct {
	endpoint  string
	sampling  float64
	batchSize int
	client    *http.Client
}

// WithEndpoint sets the OTLP HTTP endpoint URL for span export.
// If empty, the tracer operates in noop mode (spans are discarded).
func WithEndpoint(url string) TracerOption {
	return func(c *tracerConfig) {
		c.endpoint = url
	}
}

// WithSampling sets the sampling rate [0.0, 1.0].
// 1.0 samples everything, 0.0 drops everything.
func WithSampling(rate float64) TracerOption {
	return func(c *tracerConfig) {
		c.sampling = rate
	}
}

// WithBatchSize sets the number of spans to accumulate before flushing.
func WithBatchSize(n int) TracerOption {
	return func(c *tracerConfig) {
		c.batchSize = n
	}
}

// withHTTPClient sets a custom HTTP client (for testing).
func withHTTPClient(client *http.Client) TracerOption {
	return func(c *tracerConfig) {
		c.client = client
	}
}

// Tracer creates and manages spans, exporting them via OTLP HTTP JSON.
type Tracer struct {
	serviceName string
	cfg         tracerConfig
	client      *http.Client

	mu      sync.Mutex
	buffer  []*Span
	closed  bool
	sampler func() bool
}

// NewTracer creates a Tracer for the given service name.
// If no endpoint is configured, spans are silently discarded (noop mode).
func NewTracer(serviceName string, opts ...TracerOption) (*Tracer, error) {
	if serviceName == "" {
		return nil, fmt.Errorf("tracing: service name must not be empty")
	}

	cfg := tracerConfig{
		sampling:  1.0,
		batchSize: 64,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.sampling < 0 {
		cfg.sampling = 0
	}
	if cfg.sampling > 1 {
		cfg.sampling = 1
	}
	if cfg.batchSize < 1 {
		cfg.batchSize = 1
	}

	client := cfg.client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	t := &Tracer{
		serviceName: serviceName,
		cfg:         cfg,
		client:      client,
		buffer:      make([]*Span, 0, cfg.batchSize),
		sampler:     newSampler(cfg.sampling),
	}
	return t, nil
}

// StartSpan begins a new span. If a parent span exists in ctx, the new span
// inherits the trace ID and records the parent span ID.
func (t *Tracer) StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	if !t.sampler() {
		// Unsampled: return a detached span that discards on End().
		s := &Span{
			traceID:    generateTraceID(),
			spanID:     generateSpanID(),
			name:       name,
			startTime:  time.Now(),
			attributes: make(map[string]any),
			ended:      false,
			tracer:     nil, // nil tracer = noop on End()
		}
		return context.WithValue(ctx, spanContextKey{}, s), s
	}

	traceID := generateTraceID()
	var parentID string

	if parent, ok := ctx.Value(spanContextKey{}).(*Span); ok && parent != nil {
		traceID = parent.traceID
		parentID = parent.spanID
	}

	s := &Span{
		traceID:    traceID,
		spanID:     generateSpanID(),
		parentID:   parentID,
		name:       name,
		startTime:  time.Now(),
		attributes: make(map[string]any),
		tracer:     t,
	}

	return context.WithValue(ctx, spanContextKey{}, s), s
}

// SpanFromContext returns the current Span from the context, or nil.
func SpanFromContext(ctx context.Context) *Span {
	if s, ok := ctx.Value(spanContextKey{}).(*Span); ok {
		return s
	}
	return nil
}

// Shutdown flushes any buffered spans and prevents further exports.
func (t *Tracer) Shutdown(ctx context.Context) error {
	t.mu.Lock()
	t.closed = true
	pending := t.buffer
	t.buffer = nil
	t.mu.Unlock()

	if len(pending) > 0 && t.cfg.endpoint != "" {
		return t.export(ctx, pending)
	}
	return nil
}

// enqueue adds a completed span to the buffer and flushes if batch size is reached.
func (t *Tracer) enqueue(s *Span) {
	t.mu.Lock()
	if t.closed || t.cfg.endpoint == "" {
		t.mu.Unlock()
		return
	}
	t.buffer = append(t.buffer, s)
	if len(t.buffer) < t.cfg.batchSize {
		t.mu.Unlock()
		return
	}
	batch := t.buffer
	t.buffer = make([]*Span, 0, t.cfg.batchSize)
	t.mu.Unlock()

	// Fire-and-forget flush with a reasonable timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = t.export(ctx, batch)
}

// export sends spans as OTLP JSON to the configured endpoint.
func (t *Tracer) export(ctx context.Context, spans []*Span) error {
	payload := t.buildOTLPPayload(spans)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("tracing: marshal OTLP payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("tracing: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("tracing: export spans: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("tracing: collector returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// OTLP JSON wire format (minimal subset).

type otlpExportRequest struct {
	ResourceSpans []otlpResourceSpan `json:"resourceSpans"`
}

type otlpResourceSpan struct {
	Resource  otlpResource    `json:"resource"`
	ScopeSpans []otlpScopeSpan `json:"scopeSpans"`
}

type otlpResource struct {
	Attributes []otlpKeyValue `json:"attributes"`
}

type otlpScopeSpan struct {
	Scope otlpScope  `json:"scope"`
	Spans []otlpSpan `json:"spans"`
}

type otlpScope struct {
	Name string `json:"name"`
}

type otlpSpan struct {
	TraceID            string            `json:"traceId"`
	SpanID             string            `json:"spanId"`
	ParentSpanID       string            `json:"parentSpanId,omitempty"`
	Name               string            `json:"name"`
	Kind               int               `json:"kind"` // 1=INTERNAL
	StartTimeUnixNano  int64             `json:"startTimeUnixNano,string"`
	EndTimeUnixNano    int64             `json:"endTimeUnixNano,string"`
	Attributes         []otlpKeyValue    `json:"attributes,omitempty"`
	Events             []otlpSpanEvent   `json:"events,omitempty"`
	Status             *otlpStatus       `json:"status,omitempty"`
}

type otlpSpanEvent struct {
	Name              string         `json:"name"`
	TimeUnixNano      int64          `json:"timeUnixNano,string"`
	Attributes        []otlpKeyValue `json:"attributes,omitempty"`
}

type otlpStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

type otlpKeyValue struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}

type otlpValue struct {
	StringValue *string  `json:"stringValue,omitempty"`
	IntValue    *int64   `json:"intValue,omitempty,string"`
	DoubleValue *float64 `json:"doubleValue,omitempty"`
	BoolValue   *bool    `json:"boolValue,omitempty"`
}

func (t *Tracer) buildOTLPPayload(spans []*Span) otlpExportRequest {
	otelSpans := make([]otlpSpan, 0, len(spans))
	for _, s := range spans {
		s.mu.Lock()
		os := otlpSpan{
			TraceID:           s.traceID,
			SpanID:            s.spanID,
			ParentSpanID:      s.parentID,
			Name:              s.name,
			Kind:              1, // SPAN_KIND_INTERNAL
			StartTimeUnixNano: s.startTime.UnixNano(),
			EndTimeUnixNano:   s.endTime.UnixNano(),
			Attributes:        mapToOTLPAttrs(s.attributes),
		}
		if s.statusCode != StatusUnset {
			os.Status = &otlpStatus{
				Code:    int(s.statusCode),
				Message: s.statusMsg,
			}
		}
		for _, ev := range s.events {
			os.Events = append(os.Events, otlpSpanEvent{
				Name:         ev.Name,
				TimeUnixNano: ev.Timestamp.UnixNano(),
				Attributes:   mapToOTLPAttrs(ev.Attributes),
			})
		}
		s.mu.Unlock()
		otelSpans = append(otelSpans, os)
	}

	svcName := t.serviceName
	return otlpExportRequest{
		ResourceSpans: []otlpResourceSpan{
			{
				Resource: otlpResource{
					Attributes: []otlpKeyValue{
						{Key: "service.name", Value: newStringValue(svcName)},
					},
				},
				ScopeSpans: []otlpScopeSpan{
					{
						Scope: otlpScope{Name: "ralphglasses/tracing"},
						Spans: otelSpans,
					},
				},
			},
		},
	}
}

func mapToOTLPAttrs(m map[string]any) []otlpKeyValue {
	if len(m) == 0 {
		return nil
	}
	out := make([]otlpKeyValue, 0, len(m))
	for k, v := range m {
		out = append(out, otlpKeyValue{Key: k, Value: toOTLPValue(v)})
	}
	return out
}

func toOTLPValue(v any) otlpValue {
	switch val := v.(type) {
	case string:
		return newStringValue(val)
	case int:
		i := int64(val)
		return otlpValue{IntValue: &i}
	case int64:
		return otlpValue{IntValue: &val}
	case float64:
		return otlpValue{DoubleValue: &val}
	case bool:
		return otlpValue{BoolValue: &val}
	default:
		s := fmt.Sprintf("%v", v)
		return newStringValue(s)
	}
}

func newStringValue(s string) otlpValue {
	return otlpValue{StringValue: &s}
}

// ID generation using crypto/rand.

func generateTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(b)
}

func generateSpanID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}

// newSampler returns a function that returns true with probability rate.
func newSampler(rate float64) func() bool {
	if rate >= 1.0 {
		return func() bool { return true }
	}
	if rate <= 0.0 {
		return func() bool { return false }
	}
	// Use crypto/rand for sampling decisions.
	return func() bool {
		b := make([]byte, 1)
		_, _ = rand.Read(b)
		// Map byte [0,255] to [0.0, 1.0).
		return float64(b[0])/256.0 < rate
	}
}
