package nmea

const (
	// TypePASHR type for ASHR sentences
	TypePASHR = "ASHR"
)

// PASHR represents proprietary roll and pitch sentence
// https://gpsd.gitlab.io/gpsd/NMEA.html#_pashr_rt300_proprietary_roll_and_pitch_sentence
//
// Format: $-PASHR,hhmmss.sss,hhh.hh,T,rrr.rr,ppp.pp,xxx.xx,a.aaa,b.bbb,c.ccc,d,e*hh<CR><LF>
// Example: $PASHR,085335.000,224.19,T,-01.26,+00.83,+00.10,0.101,0.113,0.267,1,0*07
type PASHR struct {
	BaseSentence
	Time            Time
	Heading         float64
	TrueHeading     string
	Roll            float64
	Pitch           float64
	Heave           float64
	RollAccuracy    float64
	PitchAccuracy   float64
	HeadingAccuracy float64
	// GNSSQuality is the quality of the GNSS signal
	// The documentation differs from Trimble and Novatel about the meaning of the number.
	//
	// Trimble https://receiverhelp.trimble.com/oem-gnss/nmea0183-messages-pashr.html
	//
	// 0 – Fix not available
	//
	// 1 – GNSS mode, fix valid
	//
	// 2 – Differential GPS
	//
	// 3 – GNSS PPS mode
	//
	// 4 – Fixed RTK mode
	//
	// 5 – Float RTK mode
	//
	// 6 – DR mode
	//
	// Novatel https://docs.novatel.com/OEM7/Content/SPAN_Logs/PASHR.htm
	//
	// 0 = No position
	//
	// 1 = All non-RTK fixed integer positions
	//
	// 2 = RTK fixed integer position
	GNSSQuality int64
	// IMUAlignmentStatus is the status of the IMU alignment
	// The documentation differs from Trimble and Novatel about the meaning of the number.
	//
	// Trimble https://receiverhelp.trimble.com/oem-gnss/nmea0183-messages-pashr.html
	// 0 – GPS Only
	//
	// 1 – Coarse Leveling
	//
	// 2 – Degraded Solution
	//
	// 3 – Aligned
	//
	// 4 – Full Navigation Mode
	//
	// Novatel https://docs.novatel.com/OEM7/Content/SPAN_Logs/PASHR.htm
	//
	// 0 = All SPAN Pre-Alignment INS Status
	//
	// 1 = All SPAN Post-Alignment INS Status - These include: INS_ALIGNMENT_COMPLETE, INS_SOLUTION_GOOD, INS_HIGH_VARIANCE, INS_SOLUTION_FREE
	IMUAlignmentStatus int64
}

// newPASHR constructor
func newPASHR(s BaseSentence, opts ...ParserOption) (Sentence, error) {
	p := NewParser(s, opts...)
	p.AssertType(TypePASHR)
	return PASHR{
		BaseSentence:       s,
		Time:               p.Time(0, "time"),
		Heading:            p.Float64(1, "heading"),
		TrueHeading:        p.EnumString(2, "true  heading", "T", "F"),
		Roll:               p.Float64(3, "roll"),
		Pitch:              p.Float64(4, "pitch"),
		Heave:              p.Float64(5, "heave"),
		RollAccuracy:       p.Float64(6, "roll accuracy"),
		PitchAccuracy:      p.Float64(7, "pitch accuracy"),
		HeadingAccuracy:    p.Float64(8, "heading accuracy"),
		GNSSQuality:        p.Int64(9, "gnss quality"),
		IMUAlignmentStatus: p.Int64(10, "imu alignment status"),
	}, p.Err()
}
