// Package application defines message generation, encoding, and publishing ports.
// Generator schedules reports; AISEncoder produces NMEA; TCPPublisher and
// UDPPublisher accept ordered batches. Ports are defined here at their consumer.
// Dependencies: ais/domain, context, and time. Socket adapters live in networking.
package application
