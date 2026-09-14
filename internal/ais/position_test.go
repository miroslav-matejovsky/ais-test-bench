package ais_test

import (
	"math"
	"testing"
	"time"

	nmea "github.com/adrianmo/go-nmea"
	"github.com/miroslav-matejovsky/ais-testbench/internal/ais"
	"github.com/stretchr/testify/require"
)

func TestPositionRoundTrip(t *testing.T) {
	for i, coordinates := range [][2]float64{{54.123456, 3.654321}, {-33.876543, -70.123456}, {90, 180}, {-90, -180}} {
		channel := []ais.Channel{ais.ChannelA, ais.ChannelB}[i%2]
		p := ais.Position{MMSI: 234567890, Latitude: coordinates[0], Longitude: coordinates[1], Speed: 12.3, Course: 359.99, Heading: 359, UpdatedAt: time.Date(2026, 9, 14, 12, 30, 42, 0, time.UTC)}
		sentence, err := ais.EncodePosition(p, channel)
		require.NoError(t, err)
		require.Contains(t, sentence, "\r\n")
		parsed, err := nmea.Parse(sentence)
		require.NoError(t, err)
		report, ok := parsed.(nmea.VDMVDO)
		require.True(t, ok)
		require.EqualValues(t, 1, report.NumFragments)
		require.EqualValues(t, 1, report.FragmentNumber)
		require.Equal(t, string(channel), report.Channel)
		require.Len(t, report.Payload, 168)
		read := func(start, width int, signed bool) int64 {
			var value int64
			for _, bit := range report.Payload[start : start+width] {
				value = value*2 + int64(bit)
			}
			if signed && report.Payload[start] == 1 {
				value -= 1 << width
			}
			return value
		}
		require.EqualValues(t, 1, read(0, 6, false))
		require.EqualValues(t, p.MMSI, read(8, 30, false))
		require.EqualValues(t, 0, read(38, 4, false))
		require.EqualValues(t, -128, read(42, 8, true))
		require.EqualValues(t, 123, read(50, 10, false))
		require.InDelta(t, p.Longitude, float64(read(61, 28, true))/600000, 0.000001)
		require.InDelta(t, p.Latitude, float64(read(89, 27, true))/600000, 0.000001)
		require.EqualValues(t, 0, read(116, 12, false)) // Rounding north wraps to zero.
		require.EqualValues(t, 359, read(128, 9, false))
		require.EqualValues(t, 42, read(137, 6, false))
	}
}

func TestPositionRejectsInvalidData(t *testing.T) {
	for name, change := range map[string]func(*ais.Position){
		"mmsi":      func(p *ais.Position) { p.MMSI = 1 },
		"latitude":  func(p *ais.Position) { p.Latitude = 91 },
		"longitude": func(p *ais.Position) { p.Longitude = -181 },
		"speed":     func(p *ais.Position) { p.Speed = -1 },
		"course":    func(p *ais.Position) { p.Course = 360 },
		"heading":   func(p *ais.Position) { p.Heading = 360 },
		"nan":       func(p *ais.Position) { p.Latitude = math.NaN() },
		"infinity":  func(p *ais.Position) { p.Speed = math.Inf(1) },
		"time":      func(p *ais.Position) { p.UpdatedAt = time.Time{} },
	} {
		t.Run(name, func(t *testing.T) {
			p := ais.Position{MMSI: 234567890, UpdatedAt: time.Now()}
			change(&p)
			_, err := ais.EncodePosition(p, ais.ChannelA)
			require.Error(t, err)
		})
	}
	for _, channel := range []ais.Channel{"", "a", "1", "AB"} {
		_, err := ais.EncodePosition(ais.Position{MMSI: 234567890, UpdatedAt: time.Now()}, channel)
		require.ErrorContains(t, err, "channel")
	}
}
