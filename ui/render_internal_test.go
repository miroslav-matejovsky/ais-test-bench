package ui

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-testbench/internal/logtest"
)

type contextKey struct{}

func TestRenderFailureLogsToSuppliedLogger(t *testing.T) {
	broken := template.Must(template.New("").Parse(`{{define "document"}}{{template "absent"}}{{end}}`))
	recorder := logtest.New(slog.LevelInfo, nil)
	u := &UI{logger: slog.New(recorder).With("app", "host", "component", "ui"), pages: map[string]*template.Template{"broken": broken}}

	ctx := context.WithValue(t.Context(), contextKey{}, "request-1")
	rec := httptest.NewRecorder()
	u.render(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/broken", nil), "broken", "document", pageData{})

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Body.String(), "<", "no partial HTML is written")
	records := recorder.Records()
	require.Len(t, records, 1)
	record := records[0]
	require.Equal(t, slog.LevelError, record.Level)
	require.Equal(t, "render", record.Message)
	require.Equal(t, "host", record.Attrs["app"])
	require.Equal(t, "ui", record.Attrs["component"])
	require.Equal(t, "/broken", record.Attrs["path"])
	require.Equal(t, "document", record.Attrs["template"])
	require.ErrorContains(t, record.Attrs["error"].(error), "absent")
	require.Equal(t, "request-1", record.Context.Value(contextKey{}))
}
