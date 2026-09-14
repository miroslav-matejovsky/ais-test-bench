package application

import (
	"context"
	"time"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/ais/domain"
)

// Generator selects due reports, encodes them, and hands batches to publishers.
// Reset resets reporting cadence after starting or seeking a scenario.
type Generator interface {
	Reset(ctx context.Context, at time.Time) error
	Generate(ctx context.Context, at time.Time, reports []domain.Report) error
}

// AISEncoder validates a semantic report and returns complete NMEA lines.
// Unsupported message types and invalid values return errors; no output is published.
// Multipart fragment ordering, sequence IDs, fill bits, and checksums are AIS concerns.
type AISEncoder interface {
	Encode(ctx context.Context, report domain.Report) ([]domain.Sentence, error)
}

// TCPPublisher queues a complete ordered batch for connected TCP clients.
// Nil means locally accepted, not delivered. No clients is a valid no-op.
// Implementations copy retained data, bound queues, and disconnect slow clients
// with observable counters. Publish honors cancellation and never interleaves batches.
type TCPPublisher interface {
	Publish(ctx context.Context, sentences []domain.Sentence) error
}

// UDPPublisher queues ordered NMEA lines for all configured destinations.
// Nil means locally accepted, not remote receipt. Delivery is best effort.
// Queue saturation returns an error; each datagram contains a complete sentence.
// Implementations copy retained data and honor cancellation.
type UDPPublisher interface {
	Publish(ctx context.Context, sentences []domain.Sentence) error
}
