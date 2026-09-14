package dto

// Error is a stable API error envelope; Message is safe for display.
// Internal error details belong in logs correlated by RequestID.
type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}
