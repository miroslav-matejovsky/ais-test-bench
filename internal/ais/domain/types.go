package domain

import "time"

// Report is semantic input for a supported AIS message type.
// Validation and conversion to AIS units/sentinels belong to the AIS context.
type Report struct {
	MessageType    uint8
	MMSI           string
	Name           string
	CallSign       string
	Latitude       float64
	Longitude      float64
	SpeedKnots     float64
	CourseDegrees  float64
	HeadingDegrees float64
	// Timestamp is simulation time, never wall-clock encoding time.
	Timestamp time.Time
}

// Sentence is one complete NMEA AIS line, including checksum and trailing CRLF.
// Multipart messages are represented by ordered slices of Sentence.
type Sentence string
