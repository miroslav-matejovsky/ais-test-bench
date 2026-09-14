// Package handlers reserves REST-to-use-case translation.
// Handlers validate transport input, call management/service, visualization/service,
// and simulation/application, then map results into api/dto. Expected validation,
// not-found, and conflict errors become HTTP responses with stable codes.
// Dependencies: net/http and those inward packages. No exported declarations yet.
package handlers
