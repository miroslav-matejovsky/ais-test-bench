package display

import (
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

	"github.com/miroslav-matejovsky/ais-test-bench/internal/simulatorapi"
)

const (
	// upstreamTimeout bounds both simulator reads of one Fleet call.
	upstreamTimeout = 5 * time.Second
	// maxResponseBytes bounds each simulator response body. A fleet of 100
	// vessels is far smaller.
	maxResponseBytes = 2 << 20
)

var (
	// errUnavailable marks failures a later retry can resolve: connection
	// failures, timeouts, cancellation, and a simulator restart between reads.
	errUnavailable = errors.New("simulator unavailable")
	// errInvalid marks simulator responses that violate the simulator API.
	errInvalid = errors.New("invalid simulator response")
)

// Client reads the simulator API from one configured origin over HTTP. It is
// safe for concurrent use and reuses connections. It keeps no fleet state.
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

// Fleet reads /api/vessels and /api/metadata concurrently within
// upstreamTimeout and projects them into one complete Fleet. Both reads stop
// when ctx ends, and Fleet returns only after both have finished. Errors wrap
// errUnavailable when a retry can succeed and errInvalid when the simulator
// violated its API; no partial fleet is returned.
func (c *Client) Fleet(ctx context.Context) (Fleet, error) {
	ctx, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()

	var metadata simulatorapi.Metadata
	metadataDone := make(chan error, 1)
	go func() {
		metadataDone <- c.get(ctx, "/api/metadata", &metadata)
	}()
	var fleet simulatorapi.Fleet
	if err := c.get(ctx, "/api/vessels", &fleet); err != nil {
		// The projection needs both reads, so the metadata read is cancelled
		// and its result, at best a cancellation error, is not needed.
		cancel()
		<-metadataDone
		return Fleet{}, err
	}
	if err := <-metadataDone; err != nil {
		return Fleet{}, err
	}
	return project(fleet, metadata)
}

// get decodes one JSON object from path into value.
func (c *Client) get(ctx context.Context, path string, value any) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.origin+path, nil)
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

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: GET %s: status %s", errInvalid, path, resp.Status)
	}
	contentType := resp.Header.Get("Content-Type")
	if mediaType, _, err := mime.ParseMediaType(contentType); err != nil || mediaType != "application/json" {
		return fmt.Errorf("%w: GET %s: content type %q, want application/json", errInvalid, path, contentType)
	}
	decoder := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, maxResponseBytes))
	if err := decoder.Decode(value); err != nil {
		return bodyError(ctx, path, err)
	}
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("data after the JSON object")
		}
		return bodyError(ctx, path, err)
	}
	return nil
}

// bodyError classifies a failed body read: after cancellation or a deadline it
// is retryable, otherwise the body violates the API.
func bodyError(ctx context.Context, path string, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w: GET %s: read body: %w", errUnavailable, path, err)
	}
	return fmt.Errorf("%w: GET %s: decode body: %w", errInvalid, path, err)
}
