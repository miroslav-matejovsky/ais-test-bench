package middleware

import "net/http"

// Middleware wraps an HTTP handler. Request contexts propagate to all use cases.
type Middleware func(http.Handler) http.Handler
