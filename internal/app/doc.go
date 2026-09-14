// Package app is the composition root of the executable.
//
// Run builds the HTTP handlers from package ui, serves them on a listener
// supplied by the caller, and shuts the server down gracefully when the context
// is cancelled. This is the only internal package that wires packages together;
// business policy stays in the bounded contexts.
package app
