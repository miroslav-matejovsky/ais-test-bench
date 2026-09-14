// Package application defines vessel storage and deterministic movement ports.
// VesselRepository owns persistence access; MovementModel computes navigation.
// Dependencies: targets/domain, context, and time. Simulation orchestrates ticks.
package application
