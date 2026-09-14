package ais

import (
	"fmt"
	"math"
	"time"

	nmea "github.com/adrianmo/go-nmea"
)

// Position is a vessel's navigation report. Coordinates are decimal degrees,
// speed is knots, course and heading are degrees clockwise from true north.
type Position struct {
	MMSI      uint32    `json:"mmsi"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Speed     float64   `json:"speed"`
	Course    float64   `json:"course"`
	Heading   int       `json:"heading"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// EncodePosition encodes AIS message type 1 with underway status, unavailable
// rate of turn, and default radio state. It returns a checksummed NMEA line
// including CRLF. MMSI must have nine digits and all navigation values be valid.
func EncodePosition(p Position) (string, error) {
	if p.MMSI < 100000000 || p.MMSI > 999999999 {
		return "", fmt.Errorf("MMSI must have nine digits: %d", p.MMSI)
	}
	for _, field := range []struct {
		name            string
		value, min, max float64
	}{
		{"latitude", p.Latitude, -90, 90},
		{"longitude", p.Longitude, -180, 180},
		{"speed", p.Speed, 0, 102.2},
		{"course", p.Course, 0, math.Nextafter(360, 0)},
	} {
		if math.IsNaN(field.value) || math.IsInf(field.value, 0) || field.value < field.min || field.value > field.max {
			return "", fmt.Errorf("invalid %s: %v", field.name, field.value)
		}
	}
	if p.Heading < 0 || p.Heading >= 360 || p.UpdatedAt.IsZero() {
		return "", fmt.Errorf("invalid heading or timestamp: %d, %v", p.Heading, p.UpdatedAt)
	}

	// AIS type 1 is 168 bits, packed most significant bit first into 28
	// six-bit characters. Signed coordinates use their low two's-complement bits.
	var payload [28]byte
	bit := 0
	put := func(value int64, width int) {
		for shift := width - 1; shift >= 0; shift-- {
			payload[bit/6] |= byte((value>>shift)&1) << (5 - bit%6)
			bit++
		}
	}
	put(1, 6) // Message type.
	put(0, 2) // Repeat indicator.
	put(int64(p.MMSI), 30)
	put(0, 4)    // Underway using engine.
	put(-128, 8) // Rate of turn unavailable.
	put(int64(math.Round(p.Speed*10)), 10)
	put(0, 1) // Default position accuracy.
	put(int64(math.Round(p.Longitude*600000)), 28)
	put(int64(math.Round(p.Latitude*600000)), 27)
	put(int64(math.Round(p.Course*10))%3600, 12)
	put(int64(p.Heading), 9)
	put(int64(p.UpdatedAt.UTC().Second()), 6)
	put(0, 2+3+1+19) // Maneuver, spare, RAIM, and radio state.
	for i, value := range payload {
		payload[i] = value + 48
		if value >= 40 {
			payload[i] += 8
		}
	}
	body := "AIVDM,1,1,,A," + string(payload[:]) + ",0"
	var checksum byte
	for i := range len(body) {
		checksum ^= body[i]
	}
	sentence := fmt.Sprintf("!%s*%02X\r\n", body, checksum)
	if _, err := nmea.Parse(sentence); err != nil {
		return "", fmt.Errorf("validate generated AIS sentence: %w", err)
	}
	return sentence, nil
}
