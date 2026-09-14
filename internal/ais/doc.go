// Package ais encodes and decodes Class A position reports (AIS message type 1)
// as single-fragment !AIVDM sentences. It uses go-nmea to validate framing,
// checksum, and payload armouring.
//
// EncodePosition validates navigation inputs, converts decimal degrees and knots
// to AIS units and signed bit fields, and returns the checksummed sentence with
// CRLF. Reports use underway status, unavailable rate of turn, default
// accuracy/RAIM/radio state, and the supplied UTC second. Every generated
// sentence is parsed by go-nmea before it is returned.
//
// DecodePosition reads the type 1 fields back into a Report. It supports only
// the single-fragment AIVDM type 1 reports this module generates and rejects
// anything else with a contextual error. AIS unavailable sentinels decode to
// nil fields, so a report without a position fix never looks like 0, 0. A
// report carries only the UTC second; the full time comes from its source.
//
// The field layout follows the USCG type 1 documentation
// (https://www.navcen.uscg.gov/ais-class-a-reports).
//
// The application, domain, and infrastructure subpackages retain the earlier
// semantic report and publication contracts.
package ais
