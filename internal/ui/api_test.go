package ui_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulation"
	"github.com/miroslav-matejovsky/ais-test-bench/internal/ui"
	"github.com/stretchr/testify/require"
)

func TestLiveAPI(t *testing.T) {
	now := time.Now()
	simulator, err := simulation.New(now, 1)
	require.NoError(t, err)
	handler, err := ui.NewHandler(slog.New(slog.DiscardHandler), simulator)
	require.NoError(t, err)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	initial := request(http.MethodGet, "/api/vessels", "")
	require.Equal(t, http.StatusOK, initial.Code)
	require.Equal(t, "no-store", initial.Header().Get("Cache-Control"))
	var snapshot simulation.Snapshot
	require.NoError(t, json.Unmarshal(initial.Body.Bytes(), &snapshot))
	require.Len(t, snapshot.Vessels, 1)
	require.Equal(t, http.StatusOK, request(http.MethodPut, "/api/vessels", `{"count":3}`).Code)
	require.NoError(t, simulator.Advance(now.Add(time.Second)))
	updated := request(http.MethodGet, "/api/vessels", "")
	var live simulation.Snapshot
	require.NoError(t, json.Unmarshal(updated.Body.Bytes(), &live))
	require.Len(t, live.Vessels, 3)
	require.NotEqual(t, snapshot.Vessels[0].Latitude, live.Vessels[0].Latitude)
	var messages []simulation.Message
	require.NoError(t, json.Unmarshal(request(http.MethodGet, "/api/messages", "").Body.Bytes(), &messages))
	require.Len(t, messages, 6)
	require.Contains(t, messages[0].Sentence, "!AIVDM,")
	require.Equal(t, http.StatusOK, request(http.MethodPut, "/api/vessels", `{"count":0}`).Code)
	require.Contains(t, request(http.MethodGet, "/api/vessels", "").Body.String(), `"vessels":[]`)
	require.Equal(t, http.StatusMethodNotAllowed, request(http.MethodPost, "/api/vessels", `{"count":1}`).Code)
}

func TestCountAPIRejectsInvalidRequests(t *testing.T) {
	simulator, err := simulation.New(time.Now(), 1)
	require.NoError(t, err)
	handler, err := ui.NewHandler(slog.New(slog.DiscardHandler), simulator)
	require.NoError(t, err)
	for _, body := range []string{`{}`, `null`, `{"count":null}`, `{"count":-1}`, `{"count":101}`, `{"count":1.5}`, `{"count":"2"}`, `{"count":2,"extra":true}`, `{"count":2} {}`, `{`, strings.Repeat(" ", 1025) + `{"count":2}`} {
		t.Run(body[:min(len(body), 40)], func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/vessels", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Len(t, simulator.Snapshot().Vessels, 1)
		})
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/vessels", strings.NewReader(`{"count":2}`)))
	require.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
}
