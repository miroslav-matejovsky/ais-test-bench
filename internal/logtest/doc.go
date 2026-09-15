// Package logtest provides an in-memory slog.Handler for tests that verify
// logger injection: which logger received a record, its level, message,
// group-qualified attributes, and the context passed to the handler.
//
// Recorder applies WithAttrs and WithGroup like a structured handler and flattens
// group names into dotted attribute keys. Derived handlers share one record list.
// An optional hook runs outside the recorder lock, so tests can call back into
// the code under test from a log callback to detect records emitted under locks.
// Production code never imports this package.
package logtest
