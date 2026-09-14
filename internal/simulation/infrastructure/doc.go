// Package infrastructure reserves adapters for scenario storage, pacing,
// and lifecycle event delivery. Adapters implement simulation/application ports.
// Persistence and clock libraries belong here; domain services stay inward.
// There are no exported declarations yet; adapters will expose concrete constructors.
package infrastructure
