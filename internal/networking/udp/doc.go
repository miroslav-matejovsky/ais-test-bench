// Package udp reserves outbound AIS datagram and broadcast adapters.
// It will implement ais/application.UDPPublisher and app lifecycle contracts
// structurally, without importing app. It owns destination resolution, bounded
// queues, write deadlines, socket options, and shutdown. Dependencies: net,
// context, ais/domain, and observability adapters. No exported declarations yet.
package udp
