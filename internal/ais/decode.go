package ais

import (
	"fmt"

	nmea "github.com/adrianmo/go-nmea"
)

// Report is a decoded AIS position report. Units match Position. A nil field
// means the report marks that value unavailable.
type Report struct {
	MMSI      uint32
	Channel   Channel // Radio channel of the sentence, A or B.
	Latitude  *float64
	Longitude *float64
	Speed     *float64
	Course    *float64
	Heading   *int
	Second    *int // UTC second of the report, 0-59.
}

// AIS type 1 raw field limits and unavailable sentinels.
const (
	positionBits       = 168
	maxMMSI            = 999999999
	coordinateScale    = 600000 // Coordinates are 1/10000 minute.
	lonUnavailable     = 181 * coordinateScale
	latUnavailable     = 91 * coordinateScale
	speedUnavailable   = 1023
	courseUnavailable  = 3600
	headingUnavailable = 511
	maxSecond          = 59 // 60-63 mean no timestamp.
)

// DecodePosition decodes a single-fragment !AIVDM sentence carrying AIS message
// type 1, the reports EncodePosition produces. go-nmea validates framing,
// checksum, and payload armouring; this function decodes and validates fields.
//
// It rejects other talkers and sentence types, multipart sentences, radio
// channels other than A and B, payloads that are not 168 bits, other message
// types, an MMSI outside 1-999999999, and values outside their AIS ranges.
// Unavailable sentinels decode to nil. The position is available only when both
// coordinates are.
func DecodePosition(sentence string) (Report, error) {
	parsed, err := nmea.Parse(sentence)
	if err != nil {
		return Report{}, fmt.Errorf("parse NMEA: %w", err)
	}
	vdm, ok := parsed.(nmea.VDMVDO)
	if !ok || vdm.Talker != "AI" || vdm.Type != nmea.TypeVDM {
		return Report{}, fmt.Errorf("unsupported sentence %s: want AIVDM", parsed.Prefix())
	}
	if vdm.NumFragments != 1 || vdm.FragmentNumber != 1 {
		return Report{}, fmt.Errorf("unsupported multipart sentence: fragment %d of %d", vdm.FragmentNumber, vdm.NumFragments)
	}
	channel := Channel(vdm.Channel)
	if channel != ChannelA && channel != ChannelB {
		return Report{}, fmt.Errorf("unsupported radio channel %q: want A or B", vdm.Channel)
	}
	bits := vdm.Payload
	if len(bits) != positionBits {
		return Report{}, fmt.Errorf("payload has %d bits, want %d", len(bits), positionBits)
	}
	read := func(start, width int) int64 {
		var value int64
		for _, bit := range bits[start : start+width] {
			value = value<<1 | int64(bit)
		}
		return value
	}
	signed := func(start, width int) int64 {
		value := read(start, width)
		if bits[start] == 1 {
			value -= 1 << width
		}
		return value
	}

	if messageType := read(0, 6); messageType != 1 {
		return Report{}, fmt.Errorf("unsupported AIS message type %d", messageType)
	}
	mmsi := read(8, 30)
	if mmsi < 1 || mmsi > maxMMSI {
		return Report{}, fmt.Errorf("invalid MMSI %d", mmsi)
	}
	report := Report{MMSI: uint32(mmsi), Channel: channel}

	lon, lat := signed(61, 28), signed(89, 27)
	if lon != lonUnavailable && (lon < -180*coordinateScale || lon > 180*coordinateScale) {
		return Report{}, fmt.Errorf("MMSI %d: invalid longitude %d", mmsi, lon)
	}
	if lat != latUnavailable && (lat < -90*coordinateScale || lat > 90*coordinateScale) {
		return Report{}, fmt.Errorf("MMSI %d: invalid latitude %d", mmsi, lat)
	}
	if lon != lonUnavailable && lat != latUnavailable {
		report.Longitude = new(float64(lon) / coordinateScale)
		report.Latitude = new(float64(lat) / coordinateScale)
	}
	// Raw speed 1022 means 102.2 knots or more; it is reported as 102.2.
	if speed := read(50, 10); speed != speedUnavailable {
		report.Speed = new(float64(speed) / 10)
	}
	switch course := read(116, 12); {
	case course > courseUnavailable:
		return Report{}, fmt.Errorf("MMSI %d: invalid course %d", mmsi, course)
	case course < courseUnavailable:
		report.Course = new(float64(course) / 10)
	}
	switch heading := read(128, 9); {
	case heading == headingUnavailable:
	case heading >= 360:
		return Report{}, fmt.Errorf("MMSI %d: invalid heading %d", mmsi, heading)
	default:
		report.Heading = new(int(heading))
	}
	if second := read(137, 6); second <= maxSecond {
		report.Second = new(int(second))
	}
	return report, nil
}
