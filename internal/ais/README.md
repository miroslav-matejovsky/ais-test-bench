# AIS

The root package encodes Class A position reports (AIS type 1) into single-fragment
`!AIVDM` sentences, including checksum and CRLF. It validates navigation inputs
and checks every generated sentence with `github.com/adrianmo/go-nmea`.

Position fields use decimal degrees and knots. The encoder converts them to AIS
units and signed bit fields. Reports use underway status, unavailable rate of
turn, default accuracy/RAIM/radio state, and the supplied UTC second.
The field layout follows the [USCG type 1 documentation](https://www.navcen.uscg.gov/ais-class-a-reports).

The application/domain/infrastructure subpackages retain the earlier semantic
report and publication contracts. Package APIs are documented in `doc.go`.
