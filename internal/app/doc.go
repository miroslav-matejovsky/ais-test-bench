// Package app defines composition and lifecycle contracts for the executable.
// Bootstrapper assembles adapters and services; Runtime owns process resources.
// This is the only internal package allowed to wire all bounded contexts,
// HTTP, configuration, and embedded frontends. Business policy stays in contexts.
package app
