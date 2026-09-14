// Package api defines transport dependencies and the top-level route contract.
// Routes supplies /api/v1, /admin, /viewer, /healthz, and /metrics handlers.
// Router construction belongs here; use cases live in management, visualization,
// and simulation. Dependencies: net/http; construction may import handlers/middleware.
package api
