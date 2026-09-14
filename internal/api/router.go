package api

import "net/http"

// Routes supplies independent handlers to the router example in docs/startup.md.
// API receives paths relative to /api/v1; UI handlers receive paths relative to
// their own mount. Nil Admin or Viewer disables that mount; other fields are required.
type Routes struct {
	API     http.Handler
	Admin   http.Handler
	Viewer  http.Handler
	Health  http.Handler
	Metrics http.Handler
}
