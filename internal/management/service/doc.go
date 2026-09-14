// Package service defines management use cases for scenarios, vessels, and status.
// ScenarioManager and TargetManager validate commands before calling repositories.
// Simulator controls are supplied by simulation/application.Simulator, not duplicated.
// Dependencies: simulation/domain, targets/domain, and context.
package service
