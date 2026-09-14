// Package config defines the YAML configuration schema used by bootstrap.
// Config and section structs are data only, with time.Duration values in Go.
// The loader must reject unknown fields and validate before opening resources.
// This package depends only on the standard library.
package config
