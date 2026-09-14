// Package application defines scenario persistence and simulation use cases.
// Simulator controls lifecycle and playback. ScenarioRepository persists definitions.
// Clock, TargetStepper, TrafficGenerator, and EventBus are consumer-owned ports.
// Dependencies: simulation/domain, targets/domain, context, and time.
package application
