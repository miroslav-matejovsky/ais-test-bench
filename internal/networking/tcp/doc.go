// Package tcp reserves the AIS TCP listener and client connection adapter.
// It will implement ais/application.TCPPublisher and app lifecycle contracts
// structurally, without importing app. Responsibilities include bounded per-client
// queues, write deadlines, connection limits, and shutdown. Dependencies: net,
// context, ais/domain, and observability adapters. No exported declarations yet.
package tcp
