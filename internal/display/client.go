package display

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/miroslav-matejovsky/ais-testbench/internal/simulatorapi"
)

const (
	// upstreamTimeout bounds one simulator read, including its body.
	upstreamTimeout = 5 * time.Second
	// maxErrorBytes bounds a simulator JSON error body.
	maxErrorBytes = 64 << 10
)

var (
	// errUnavailable marks failures a later retry can resolve: connection
	// failures, timeouts, and cancellation.
	errUnavailable = errors.New("simulator unavailable")
	// errInvalid marks simulator responses that violate the simulator API.
	errInvalid = errors.New("invalid simulator response")
)

// statusError is a simulator API error the browser can act on: 400 for a
// rejected query, 404 for an unknown station, or 409 for another run.
type statusError struct {
	status int
	// message is the simulator's error text.
	message string
}

func (e *statusError) Error() string {
	return fmt.Sprintf("simulator status %d: %s", e.status, e.message)
}

// Client reads the simulator API from one configured origin over HTTP. It is
// safe for concurrent use and reuses connections. It keeps no simulator state.
type Client struct {
	origin string // scheme://host[:port]
	http   *http.Client
}

// NewClient returns a client for the simulator at simulatorURL: an http or
// https origin with a host, an optional port, and an empty or "/" path. User
// info, query, fragment, and other paths are rejected.
func NewClient(simulatorURL string) (*Client, error) {
	origin, err := parseOrigin(simulatorURL)
	if err != nil {
		return nil, fmt.Errorf("invalid simulator URL %q: %w", simulatorURL, err)
	}
	// A private transport lets CloseIdleConnections affect only this client.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	return &Client{origin: origin, http: &http.Client{Transport: transport}}, nil
}

func parseOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return "", errors.New("scheme must be http or https")
	case u.Opaque != "" || u.Hostname() == "":
		return "", errors.New("host is required")
	case u.User != nil:
		return "", errors.New("user info is not allowed")
	case strings.ContainsAny(raw, "?#"):
		return "", errors.New("query and fragment are not allowed")
	case u.Path != "" && u.Path != "/":
		return "", errors.New("path is not allowed")
	case strings.HasSuffix(u.Host, ":"):
		return "", errors.New("port is empty")
	}
	if port := u.Port(); port != "" {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("port %q out of range 1-65535", port)
		}
	}
	return u.Scheme + "://" + u.Host, nil
}

// Origin returns the simulator origin the client reads: scheme://host[:port].
func (c *Client) Origin() string {
	return c.origin
}

// CloseIdleConnections closes idle simulator connections, for shutdown.
func (c *Client) CloseIdleConnections() {
	c.http.CloseIdleConnections()
}

// Observations reads one GET /api/observations snapshot within upstreamTimeout
// and projects it. stations selects station IDs; nil selects all stations.
// Errors wrap errUnavailable when a retry can succeed, *statusError for a
// simulator 400 or 404, and errInvalid when the simulator violated its API.
// No partial projection is returned.
func (c *Client) Observations(ctx context.Context, stations []string) (Observations, error) {
	selector := "all"
	if stations != nil {
		selector = strings.Join(stations, ",")
	}
	query := url.Values{"stations": {selector}}
	var upstream upstreamObservations
	if err := c.get(ctx, "/api/observations", query, simulatorapi.ObservationResponseLimit, &upstream); err != nil {
		return Observations{}, err
	}
	o, err := projectObservations(stations, upstream)
	if err != nil {
		return Observations{}, fmt.Errorf("%w: GET /api/observations: %w", errInvalid, err)
	}
	return o, nil
}

// HistoryRequest selects one page of a station's reception history.
type HistoryRequest struct {
	// SimulationID is the run the cursor belongs to; required.
	SimulationID string
	// After is the last reception sequence already read; nil requests the
	// newest receptions.
	After *uint64
	// Limit is the page size, 1-simulatorapi.ReceptionPageLimit; 0 uses the
	// simulator default.
	Limit int
}

// ReceptionHistory reads one GET /api/stations/{id}/receptions page within
// upstreamTimeout and projects it. Errors are classified as for Observations;
// a *statusError can also carry 409 when req.SimulationID is not the current run.
func (c *Client) ReceptionHistory(ctx context.Context, stationID string, req HistoryRequest) (ReceptionPage, error) {
	query := url.Values{"simulationId": {req.SimulationID}}
	if req.After != nil {
		query.Set("after", strconv.FormatUint(*req.After, 10))
	}
	if req.Limit != 0 {
		query.Set("limit", strconv.Itoa(req.Limit))
	}
	path := "/api/stations/" + url.PathEscape(stationID) + "/receptions"
	var upstream simulatorapi.ReceptionPage
	if err := c.get(ctx, path, query, simulatorapi.ReceptionResponseLimit, &upstream); err != nil {
		return ReceptionPage{}, err
	}
	page, err := projectReceptionPage(stationID, req, upstream)
	if err != nil {
		return ReceptionPage{}, fmt.Errorf("%w: GET %s: %w", errInvalid, path, err)
	}
	return page, nil
}

// get reads one JSON object from path within upstreamTimeout. It checks the
// status and content type, bounds the body to limit bytes before decoding,
// checks decimal uint64 strings, and decodes the object into value.
func (c *Client) get(ctx context.Context, path string, query url.Values, limit int64, value any) (err error) {
	ctx, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.origin+path+"?"+query.Encode(), nil)
	if err != nil {
		return fmt.Errorf("create request GET %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: GET %s: %w", errUnavailable, path, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close GET %s response: %w", path, closeErr))
		}
	}()

	contentType := resp.Header.Get("Content-Type")
	if mediaType, _, err := mime.ParseMediaType(contentType); err != nil || mediaType != "application/json" {
		return fmt.Errorf("%w: GET %s: status %s, content type %q, want application/json", errInvalid, path, resp.Status, contentType)
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict:
		body, err := readBody(ctx, path, resp.Body, maxErrorBytes)
		if err != nil {
			return err
		}
		var problem simulatorapi.APIError
		if err := json.Unmarshal(body, &problem); err != nil || problem.Error == "" {
			return fmt.Errorf("%w: GET %s: status %s without an API error: %s", errInvalid, path, resp.Status, body)
		}
		return fmt.Errorf("GET %s: %w", path, &statusError{status: resp.StatusCode, message: problem.Error})
	default:
		return fmt.Errorf("%w: GET %s: unexpected status %s", errInvalid, path, resp.Status)
	}
	body, err := readBody(ctx, path, resp.Body, limit)
	if err != nil {
		return err
	}
	if err := checkDecimalStrings(body); err != nil {
		return fmt.Errorf("%w: GET %s: %w", errInvalid, path, err)
	}
	if err := json.Unmarshal(body, value); err != nil {
		return fmt.Errorf("%w: GET %s: decode body: %w", errInvalid, path, err)
	}
	return nil
}

// readBody reads at most limit bytes. A longer body violates the API. A read
// failure after cancellation or a deadline is retryable.
func readBody(ctx context.Context, path string, body io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: GET %s: read body: %w", errUnavailable, path, err)
		}
		return nil, fmt.Errorf("%w: GET %s: read body: %w", errInvalid, path, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: GET %s: body exceeds %d bytes", errInvalid, path, limit)
	}
	return data, nil
}

// decimalFields are the simulator JSON keys whose string values encode uint64
// identities, revisions, cursors, and counters.
var decimalFields = map[string]bool{
	"stateRevision": true, "stationSetRevision": true, "configRevision": true, "rfRevision": true,
	"sequence": true, "transmissionSequence": true, "transmissions": true, "receivedTransmissions": true,
	"receptions": true, "targetEvictions": true, "opportunities": true, "received": true,
	"stationDisabled": true, "channelDisabled": true, "outsideHorizon": true, "insufficientMargin": true,
	"probabilisticLoss": true, "oldestReception": true, "latestReception": true, "oldestAvailable": true,
	"latestAvailable": true, "truncatedBefore": true, "nextAfter": true,
}

// checkDecimalStrings scans a JSON document and requires every string value of
// a decimalFields key to be a canonical unsigned decimal uint64: no sign,
// leading zero, exponent, or overflow. encoding/json alone accepts leading
// zeros in ",string" fields. Non-string values are left to the typed decode.
func checkDecimalStrings(body []byte) error {
	type frame struct{ object, expectKey bool }
	var stack []frame
	key := ""
	decoder := json.NewDecoder(bytes.NewReader(body))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("scan JSON: %w", err)
		}
		n := len(stack)
		switch t := token.(type) {
		case json.Delim:
			switch t {
			case '{':
				stack = append(stack, frame{object: true, expectKey: true})
				continue
			case '[':
				stack = append(stack, frame{})
				continue
			}
			stack = stack[:n-1]
			n--
		case string:
			if n > 0 && stack[n-1].object && stack[n-1].expectKey {
				key = t
				stack[n-1].expectKey = false
				continue
			}
			if n > 0 && stack[n-1].object && decimalFields[key] {
				if _, err := parseDecimal(t); err != nil {
					return fmt.Errorf("%s: %w", key, err)
				}
			}
		}
		// A value, including a closed container, completes an object member.
		if n > 0 && stack[n-1].object {
			stack[n-1].expectKey = true
		}
	}
}

// parseDecimal parses a canonical unsigned decimal uint64 string. It is the
// display's single parser for simulator uint64 identities and cursors.
func parseDecimal(s string) (uint64, error) {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid unsigned decimal %q: %w", s, err)
	}
	if strconv.FormatUint(n, 10) != s {
		return 0, fmt.Errorf("noncanonical unsigned decimal %q", s)
	}
	return n, nil
}
