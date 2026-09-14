package ais_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/ais-test-bench/internal/ais"
)

// coordinateTolerance is one AIS coordinate unit, 1/600000 degree.
const coordinateTolerance = 1.0 / 600000

// type1 holds raw AIS type 1 field values in AIS units, independent of the
// package encoder.
type type1 struct {
	msgType, mmsi, speed, lon, lat, course, heading, second int64
	extraBits                                               int // Added to (negative: removed from) the 168 bits; a multiple of 6.
}

// validType1 is 52.5 N, 4.25 E, 12.3 knots, course 90.0, heading 90, second 42.
func validType1() type1 {
	return type1{msgType: 1, mmsi: 234567890, speed: 123, lon: 2550000, lat: 31500000, course: 900, heading: 90, second: 42}
}

// payload packs the fields most significant bit first and armours them.
func (f type1) payload() string {
	bits := make([]byte, 0, 168)
	put := func(value int64, width int) {
		for shift := width - 1; shift >= 0; shift-- {
			bits = append(bits, byte(value>>shift&1))
		}
	}
	put(f.msgType, 6)
	put(0, 2)
	put(f.mmsi, 30)
	put(0, 4)
	put(-128, 8)
	put(f.speed, 10)
	put(0, 1)
	put(f.lon, 28)
	put(f.lat, 27)
	put(f.course, 12)
	put(f.heading, 9)
	put(f.second, 6)
	put(0, 25)
	if f.extraBits < 0 {
		bits = bits[:len(bits)+f.extraBits]
	} else {
		bits = append(bits, make([]byte, f.extraBits)...)
	}
	armoured := make([]byte, 0, len(bits)/6)
	for i := 0; i+6 <= len(bits); i += 6 {
		var value byte
		for _, bit := range bits[i : i+6] {
			value = value<<1 | bit
		}
		if value >= 40 {
			value += 8
		}
		armoured = append(armoured, value+48)
	}
	return string(armoured)
}

func (f type1) sentence() string {
	return frame("AIVDM,1,1,,A," + f.payload() + ",0")
}

// frame adds the start character, checksum, and CRLF to an NMEA body.
func frame(body string) string {
	var checksum byte
	for i := range len(body) {
		checksum ^= body[i]
	}
	return fmt.Sprintf("!%s*%02X\r\n", body, checksum)
}

func TestDecodeKnownReports(t *testing.T) {
	// Published sample reports from the gpsd AIVDM documentation. Published
	// coordinates are shortened to 6 or 5 decimals, so tolerance is one AIS
	// coordinate unit or the published precision.
	tests := []struct {
		sentence            string
		channel             ais.Channel
		mmsi                uint32
		latitude, longitude float64
		tolerance           float64
		speed, course       float64
		heading, second     int
	}{
		{sentence: "!AIVDM,1,1,,B,177KQJ5000G?tO`K>RA1wUbN0TKH,0*5C", channel: ais.ChannelB, mmsi: 477553000, latitude: 47.582833, longitude: -122.345832, tolerance: coordinateTolerance, speed: 0, course: 51, heading: 181, second: 15},
		{sentence: "!AIVDM,1,1,,A,15RTgt0PAso;90TKcjM8h6g208CQ,0*4A", channel: ais.ChannelA, mmsi: 371798000, latitude: 48.38163, longitude: -123.395383, tolerance: 1e-5, speed: 12.3, course: 224, heading: 215, second: 33},
	}
	for _, tt := range tests {
		t.Run(tt.sentence, func(t *testing.T) {
			report, err := ais.DecodePosition(tt.sentence)
			require.NoError(t, err)
			require.Equal(t, tt.channel, report.Channel)
			require.Equal(t, tt.mmsi, report.MMSI)
			require.InDelta(t, tt.latitude, *report.Latitude, tt.tolerance)
			require.InDelta(t, tt.longitude, *report.Longitude, tt.tolerance)
			require.InDelta(t, tt.speed, *report.Speed, 1e-9)
			require.InDelta(t, tt.course, *report.Course, 1e-9)
			require.Equal(t, tt.heading, *report.Heading)
			require.Equal(t, tt.second, *report.Second)
		})
	}
}

func TestDecodeFieldBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		change func(*type1)
		check  func(*testing.T, ais.Report)
	}{
		{"valid", func(*type1) {}, func(t *testing.T, r ais.Report) {
			require.Equal(t, uint32(234567890), r.MMSI)
			require.InDelta(t, 52.5, *r.Latitude, coordinateTolerance)
			require.InDelta(t, 4.25, *r.Longitude, coordinateTolerance)
			require.InDelta(t, 12.3, *r.Speed, 1e-9)
			require.InDelta(t, 90.0, *r.Course, 1e-9)
			require.Equal(t, 90, *r.Heading)
			require.Equal(t, 42, *r.Second)
		}},
		{"negative coordinates", func(f *type1) { f.lat, f.lon = -20000000, -100000000 }, func(t *testing.T, r ais.Report) {
			require.InDelta(t, -33.333333, *r.Latitude, coordinateTolerance)
			require.InDelta(t, -166.666667, *r.Longitude, coordinateTolerance)
		}},
		{"minimum values", func(f *type1) {
			f.lat, f.lon, f.speed, f.course, f.heading, f.second, f.mmsi = -54000000, -108000000, 0, 0, 0, 0, 1
		}, func(t *testing.T, r ais.Report) {
			require.Equal(t, uint32(1), r.MMSI)
			require.InDelta(t, -90.0, *r.Latitude, coordinateTolerance)
			require.InDelta(t, -180.0, *r.Longitude, coordinateTolerance)
			require.Zero(t, *r.Speed)
			require.Zero(t, *r.Course)
			require.Zero(t, *r.Heading)
			require.Zero(t, *r.Second)
		}},
		{"maximum values", func(f *type1) {
			f.lat, f.lon, f.speed, f.course, f.heading, f.second, f.mmsi = 54000000, 108000000, 1022, 3599, 359, 59, 999999999
		}, func(t *testing.T, r ais.Report) {
			require.Equal(t, uint32(999999999), r.MMSI)
			require.InDelta(t, 90.0, *r.Latitude, coordinateTolerance)
			require.InDelta(t, 180.0, *r.Longitude, coordinateTolerance)
			require.InDelta(t, 102.2, *r.Speed, 1e-9)
			require.InDelta(t, 359.9, *r.Course, 1e-9)
			require.Equal(t, 359, *r.Heading)
			require.Equal(t, 59, *r.Second)
		}},
		{"all unavailable", func(f *type1) {
			f.lat, f.lon, f.speed, f.course, f.heading, f.second = 54600000, 108600000, 1023, 3600, 511, 60
		}, func(t *testing.T, r ais.Report) {
			require.Equal(t, ais.Report{MMSI: 234567890, Channel: ais.ChannelA}, r)
		}},
		{"longitude unavailable hides position", func(f *type1) { f.lon = 108600000 }, func(t *testing.T, r ais.Report) {
			require.Nil(t, r.Latitude)
			require.Nil(t, r.Longitude)
			require.NotNil(t, r.Speed)
		}},
		{"latitude unavailable hides position", func(f *type1) { f.lat = 54600000 }, func(t *testing.T, r ais.Report) {
			require.Nil(t, r.Latitude)
			require.Nil(t, r.Longitude)
		}},
		{"timestamp not available codes", func(f *type1) { f.second = 63 }, func(t *testing.T, r ais.Report) {
			require.Nil(t, r.Second)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := validType1()
			tt.change(&f)
			report, err := ais.DecodePosition(f.sentence())
			require.NoError(t, err)
			tt.check(t, report)
		})
	}
}

func TestDecodeEncoderRoundTrip(t *testing.T) {
	for i, coordinates := range [][2]float64{{54.123456, 3.654321}, {-33.876543, -70.123456}, {90, 180}, {-90, -180}, {0, 0}} {
		channel := []ais.Channel{ais.ChannelA, ais.ChannelB}[i%2]
		p := ais.Position{MMSI: 234567890, Latitude: coordinates[0], Longitude: coordinates[1], Speed: 12.3, Course: 123.4, Heading: 359, UpdatedAt: time.Date(2026, 9, 14, 12, 30, 42, 0, time.UTC)}
		sentence, err := ais.EncodePosition(p, channel)
		require.NoError(t, err)

		report, err := ais.DecodePosition(sentence)

		require.NoError(t, err)
		require.Equal(t, channel, report.Channel)
		require.Equal(t, p.MMSI, report.MMSI)
		require.InDelta(t, p.Latitude, *report.Latitude, coordinateTolerance)
		require.InDelta(t, p.Longitude, *report.Longitude, coordinateTolerance)
		require.InDelta(t, p.Speed, *report.Speed, 0.05)
		require.InDelta(t, p.Course, *report.Course, 0.05)
		require.Equal(t, p.Heading, *report.Heading)
		require.Equal(t, 42, *report.Second)
	}
}

func TestDecodeRejectsUnsupportedReports(t *testing.T) {
	valid := validType1()
	with := func(change func(*type1)) string {
		f := validType1()
		change(&f)
		return f.sentence()
	}
	for name, sentence := range map[string]string{
		"empty":               "",
		"not NMEA":            "hello",
		"corrupt checksum":    strings.Replace(valid.sentence(), ",A,", ",B,", 1),
		"multipart":           frame("AIVDM,2,1,7,A," + valid.payload() + ",0"),
		"own vessel VDO":      frame("AIVDO,1,1,,A," + valid.payload() + ",0"),
		"other talker":        frame("BSVDM,1,1,,A," + valid.payload() + ",0"),
		"fill bits":           frame("AIVDM,1,1,,A," + valid.payload() + ",2"),
		"missing channel":     frame("AIVDM,1,1,,," + valid.payload() + ",0"),
		"numeric channel":     frame("AIVDM,1,1,,1," + valid.payload() + ",0"),
		"short payload":       with(func(f *type1) { f.extraBits = -6 }),
		"long payload":        with(func(f *type1) { f.extraBits = 6 }),
		"message type 2":      with(func(f *type1) { f.msgType = 2 }),
		"message type 5":      with(func(f *type1) { f.msgType = 5 }),
		"zero MMSI":           with(func(f *type1) { f.mmsi = 0 }),
		"ten digit MMSI":      with(func(f *type1) { f.mmsi = 1000000000 }),
		"longitude range":     with(func(f *type1) { f.lon = 108000001 }),
		"latitude range":      with(func(f *type1) { f.lat = -54000001 }),
		"course range":        with(func(f *type1) { f.course = 3601 }),
		"heading 360":         with(func(f *type1) { f.heading = 360 }),
		"heading 510":         with(func(f *type1) { f.heading = 510 }),
		"invalid lat, no lon": with(func(f *type1) { f.lon, f.lat = 108600000, 60000000 }),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ais.DecodePosition(sentence)
			require.Error(t, err)
		})
	}
}
