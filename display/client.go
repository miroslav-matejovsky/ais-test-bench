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

	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
)

const (
	// upstreamTimeout bounds one simulator read, including its body.
	upstreamTimeout = 5 * time.Second
	// maxErrorBytes bounds a simulator JSON error body.
	maxErrorBytes = 64 << 10
)

// HTTPConfig selects the simulator origin and optional caller-owned HTTP client.
// Origin accepts HTTP(S) scheme://host[:port], without a path other than "/".
type HTTPConfig struct {
	Origin string
	// Client is borrowed and is never mutated or closed. Nil creates an owned transport.
	Client *http.Client
}

// HTTPSource reads received-traffic wire snapshots over HTTP. It is safe for
// concurrent use, starts no background work, and implements Source. Consumers
// own its lifetime and call CloseIdleConnections when they finish using it.
type HTTPSource struct {
	origin string
	http   *http.Client
	owned  bool
}

// NewHTTPSource validates the origin without connecting. Each read has a
// five-second deadline, observes caller cancellation, and bounds its JSON body.
func NewHTTPSource(config HTTPConfig) (*HTTPSource, error) {
	origin, err := parseOrigin(config.Origin)
	if err != nil {
		return nil, fmt.Errorf("invalid simulator URL %q: %w", config.Origin, err)
	}
	client := config.Client
	owned := client == nil
	if owned {
		defaultTransport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, errors.New("default transport cannot be cloned; provide HTTPConfig.Client")
		}
		transport := defaultTransport.Clone()
		client = &http.Client{Transport: transport}
	}
	return &HTTPSource{origin: origin, http: client, owned: owned}, nil
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
func (c *HTTPSource) Origin() string {
	return c.origin
}

// CloseIdleConnections closes idle simulator connections, for shutdown.
func (c *HTTPSource) CloseIdleConnections() {
	if c.owned {
		c.http.CloseIdleConnections()
	}
}

// Observations reads GET /api/observations. It checks JSON framing, required
// clock field presence, and canonical decimal strings. Semantic validation and
// NMEA decoding are performed by Client for every Source, including this one.
func (c *HTTPSource) Observations(ctx context.Context, stations []string) (simulatorapi.Observations, error) {
	if err := sourceContext(ctx); err != nil {
		return simulatorapi.Observations{}, err
	}
	if err := simulatorapi.ValidateSelection(stations); err != nil {
		return simulatorapi.Observations{}, err
	}
	selector := "all"
	if stations != nil {
		selector = strings.Join(stations, ",")
	}
	query := url.Values{"stations": {selector}}
	var upstream upstreamObservations
	if err := c.get(ctx, "/api/observations", query, simulatorapi.ObservationResponseLimit, &upstream); err != nil {
		return simulatorapi.Observations{}, err
	}
	clock := upstream.Time
	if clock == nil || clock.Now == nil || clock.ElapsedMs == nil || clock.Speed == nil || clock.Paused == nil {
		return simulatorapi.Observations{}, fmt.Errorf("%w: time.now, time.elapsedMs, time.speed, and time.paused are required", simulatorapi.ErrInvalidResponse)
	}
	upstream.Observations.Time = simulatorapi.TimeState{Now: *clock.Now, ElapsedMs: *clock.ElapsedMs, Speed: *clock.Speed, Paused: *clock.Paused}
	if err := sourceContext(ctx); err != nil {
		return simulatorapi.Observations{}, err
	}
	return upstream.Observations, nil
}

// ReceptionHistory reads a bounded GET /api/stations/{id}/receptions wire page.
// It returns source error categories from simulatorapi, retaining original causes.
func (c *HTTPSource) ReceptionHistory(ctx context.Context, stationID string, req simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
	if err := sourceContext(ctx); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	if err := req.Validate(); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	if err := simulatorapi.ValidateSelection([]string{stationID}); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
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
		return simulatorapi.ReceptionPage{}, err
	}
	if err := sourceContext(ctx); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	return upstream, nil
}

// get reads one JSON object from path within upstreamTimeout. It checks the
// status and content type, bounds the body to limit bytes before decoding,
// checks decimal uint64 strings, and decodes the object into value.
func (c *HTTPSource) get(ctx context.Context, path string, query url.Values, limit int64, value any) (err error) {
	ctx, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.origin+path+"?"+query.Encode(), nil)
	if err != nil {
		return fmt.Errorf("create request GET %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: GET %s: %w", simulatorapi.ErrUnavailable, path, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close GET %s response: %w", path, closeErr))
		}
	}()

	contentType := resp.Header.Get("Content-Type")
	if mediaType, _, err := mime.ParseMediaType(contentType); err != nil || mediaType != "application/json" {
		return fmt.Errorf("%w: GET %s: status %s, content type %q, want application/json", simulatorapi.ErrInvalidResponse, path, resp.Status, contentType)
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
			return fmt.Errorf("%w: GET %s: status %s without an API error: %s", simulatorapi.ErrInvalidResponse, path, resp.Status, body)
		}
		kind := simulatorapi.ErrInvalidRequest
		switch resp.StatusCode {
		case http.StatusNotFound:
			kind = simulatorapi.ErrNotFound
		case http.StatusConflict:
			kind = simulatorapi.ErrConflict
		}
		return fmt.Errorf("GET %s: %w: %s", path, kind, problem.Error)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("%w: GET %s: status %s", simulatorapi.ErrUnavailable, path, resp.Status)
	default:
		return fmt.Errorf("%w: GET %s: unexpected status %s", simulatorapi.ErrInvalidResponse, path, resp.Status)
	}
	body, err := readBody(ctx, path, resp.Body, limit)
	if err != nil {
		return err
	}
	if err := checkDecimalStrings(body); err != nil {
		return fmt.Errorf("%w: GET %s: %w", simulatorapi.ErrInvalidResponse, path, err)
	}
	if err := json.Unmarshal(body, value); err != nil {
		return fmt.Errorf("%w: GET %s: decode body: %w", simulatorapi.ErrInvalidResponse, path, err)
	}
	return nil
}

// readBody reads at most limit bytes. A longer body violates the API. A read
// failure after cancellation or a deadline is retryable.
func readBody(ctx context.Context, path string, body io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: GET %s: read body: %w", simulatorapi.ErrUnavailable, path, err)
		}
		return nil, fmt.Errorf("%w: GET %s: read body: %w", simulatorapi.ErrInvalidResponse, path, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: GET %s: body exceeds %d bytes", simulatorapi.ErrInvalidResponse, path, limit)
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
