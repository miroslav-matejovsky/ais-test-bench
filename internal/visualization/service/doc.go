// Package service defines viewer queries and chart-provider boundaries.
// Viewer returns projected target snapshots; TargetReader supplies domain state;
// ChartCatalog describes local chart sources. Playback commands use the shared
// simulation application, so the viewer never creates a second simulation.
// Dependencies: targets/domain, context, and time.
package service
