package logtest

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"sync"
)

// Record is one captured log record.
type Record struct {
	Level   slog.Level
	Message string
	// Attrs holds resolved attribute values keyed by their group-qualified name,
	// for example "host.app" for attribute "app" inside group "host".
	Attrs map[string]any
	// Context is the context passed to Handle.
	Context context.Context
}

// Recorder is a slog.Handler that captures records in memory. Handlers derived
// with WithAttrs and WithGroup share the captured records. It is safe for
// concurrent use. Construct it with New.
type Recorder struct {
	shared *shared
	level  slog.Level
	prefix string         // Group prefix ending in "." or empty.
	attrs  map[string]any // Attributes added by WithAttrs, qualified.
}

type shared struct {
	mu      sync.Mutex
	records []Record
	hook    func(context.Context, Record)
}

// New returns a recorder enabled at level and above. A non-nil hook runs after
// each record is captured, outside the recorder's lock.
func New(level slog.Level, hook func(context.Context, Record)) *Recorder {
	return &Recorder{shared: &shared{hook: hook}, level: level, attrs: map[string]any{}}
}

// Records returns a copy of the captured records in handling order.
func (h *Recorder) Records() []Record {
	h.shared.mu.Lock()
	defer h.shared.mu.Unlock()
	return slices.Clone(h.shared.records)
}

// Enabled reports whether level is at least the recorder level.
func (h *Recorder) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle captures r with the recorder's attributes and groups applied.
func (h *Recorder) Handle(ctx context.Context, r slog.Record) error {
	record := Record{Level: r.Level, Message: r.Message, Attrs: maps.Clone(h.attrs), Context: ctx}
	r.Attrs(func(a slog.Attr) bool {
		add(record.Attrs, h.prefix, a)
		return true
	})
	h.shared.mu.Lock()
	h.shared.records = append(h.shared.records, record)
	h.shared.mu.Unlock()
	if h.shared.hook != nil {
		h.shared.hook(ctx, record)
	}
	return nil
}

// WithAttrs returns a recorder adding attrs within the current groups.
func (h *Recorder) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = maps.Clone(h.attrs)
	for _, a := range attrs {
		add(next.attrs, h.prefix, a)
	}
	return &next
}

// WithGroup returns a recorder qualifying later attributes with name.
func (h *Recorder) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := *h
	next.prefix = h.prefix + name + "."
	return &next
}

// add stores a resolved attribute, flattening groups into qualified keys.
func add(attrs map[string]any, prefix string, a slog.Attr) {
	value := a.Value.Resolve()
	if value.Kind() != slog.KindGroup {
		if a.Key != "" {
			attrs[prefix+a.Key] = value.Any()
		}
		return
	}
	groupPrefix := prefix
	if a.Key != "" {
		groupPrefix = prefix + a.Key + "."
	}
	for _, member := range value.Group() {
		add(attrs, groupPrefix, member)
	}
}
