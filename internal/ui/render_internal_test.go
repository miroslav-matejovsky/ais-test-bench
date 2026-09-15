package ui

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/internal/logtest"
)

type contextKey struct{}

func TestRenderFailureLogsToSuppliedLogger(t *testing.T) {
	templates := fstest.MapFS{"base.tmpl": {Data: []byte(`{{define "base"}}{{template "absent"}}{{end}}`)}}
	html, err := newRenderer(templates, nil, "base.tmpl")
	require.NoError(t, err)
	recorder := logtest.New(slog.LevelInfo, nil)
	pages := &Pages{logger: slog.New(recorder).With("app", "host", "component", "ui"), html: html}

	ctx := context.WithValue(t.Context(), contextKey{}, "request-1")
	rec := httptest.NewRecorder()
	pages.render(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/broken", nil), nil, "base")

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	records := recorder.Records()
	require.Len(t, records, 1)
	record := records[0]
	require.Equal(t, slog.LevelError, record.Level)
	require.Equal(t, "render", record.Message)
	require.Equal(t, "host", record.Attrs["app"])
	require.Equal(t, "ui", record.Attrs["component"])
	require.Equal(t, "/broken", record.Attrs["path"])
	require.Equal(t, "base", record.Attrs["template"])
	require.ErrorContains(t, record.Attrs["error"].(error), "absent")
	require.Equal(t, "request-1", record.Context.Value(contextKey{}))
}
