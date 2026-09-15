package simulator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/miroslav-matejovsky/ais-testbench/simulation"
	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
)

func (a *api) stations(w http.ResponseWriter, r *http.Request) {
	if _, err := queryValues(r); err != nil {
		a.stationError(w, r, err)
		return
	}
	a.writeJSON(w, r, stationSetResponse(a.sim.Stations()))
}

func (a *api) addStation(w http.ResponseWriter, r *http.Request) {
	input, revision, definition, ok := a.readStation(w, r)
	if !ok {
		return
	}
	id, set, err := a.sim.AddStation(r.Context(), input.SimulationID, revision, definition)
	if err != nil {
		a.stationError(w, r, err)
		return
	}
	// A relative reference resolves against the request URL, so it keeps any
	// public prefix a host or proxy stripped before this handler.
	w.Header().Set("Location", "stations/"+url.PathEscape(id))
	a.writeStatusJSON(w, r, http.StatusCreated, simulatorapi.StationCreated{StationSet: stationSetResponse(set), StationID: id})
}

func (a *api) updateStation(w http.ResponseWriter, r *http.Request) {
	input, revision, definition, ok := a.readStation(w, r)
	if !ok {
		return
	}
	set, err := a.sim.UpdateStation(r.Context(), input.SimulationID, revision, r.PathValue("id"), definition)
	if err != nil {
		a.stationError(w, r, err)
		return
	}
	a.writeJSON(w, r, stationSetResponse(set))
}

func (a *api) removeStation(w http.ResponseWriter, r *http.Request) {
	values, err := queryValues(r, "simulationId", "stationSetRevision")
	if err != nil {
		a.stationError(w, r, err)
		return
	}
	id, revision, err := editIdentity(values.Get("simulationId"), values.Get("stationSetRevision"))
	if err != nil {
		a.stationError(w, r, err)
		return
	}
	set, err := a.sim.RemoveStation(r.Context(), id, revision, r.PathValue("id"))
	if err != nil {
		a.stationError(w, r, err)
		return
	}
	a.writeJSON(w, r, stationSetResponse(set))
}

func (a *api) observations(w http.ResponseWriter, r *http.Request) {
	values, err := queryValues(r, "stations")
	if err != nil {
		a.stationError(w, r, err)
		return
	}
	var selection []string
	if values.Has("stations") && values.Get("stations") != "all" {
		selection = strings.Split(values.Get("stations"), ",")
		if len(selection) > simulation.MaxStations {
			a.stationError(w, r, invalid("too many selected stations"))
			return
		}
		seen := make(map[string]bool)
		for _, id := range selection {
			if id == "" || strings.TrimSpace(id) != id || id == "all" || seen[id] {
				a.stationError(w, r, invalid("stations must be all or distinct, nonempty station IDs"))
				return
			}
			seen[id] = true
		}
	}
	o, err := a.sim.Observations(r.Context(), selection)
	if err != nil {
		a.stationError(w, r, err)
		return
	}
	a.writeJSON(w, r, o)
}

func (a *api) receptions(w http.ResponseWriter, r *http.Request) {
	values, err := queryValues(r, "simulationId", "after", "limit")
	if err != nil {
		a.stationError(w, r, err)
		return
	}
	if values.Get("simulationId") == "" {
		a.stationError(w, r, invalid("simulationId is required"))
		return
	}
	limit := uint64(100)
	if values.Has("limit") {
		limit, err = decimalUint(values.Get("limit"))
		if err != nil || limit < 1 || limit > simulatorapi.ReceptionPageLimit {
			a.stationError(w, r, invalid("limit must be an integer from 1 through 200"))
			return
		}
	}
	var after *uint64
	if values.Has("after") {
		n, parseErr := decimalUint(values.Get("after"))
		if parseErr != nil {
			a.stationError(w, r, invalid("invalid after cursor: "+parseErr.Error()))
			return
		}
		after = &n
	}
	page, err := a.sim.ReceptionHistory(r.Context(), r.PathValue("id"), simulatorapi.HistoryRequest{SimulationID: values.Get("simulationId"), After: after, Limit: int(limit)})
	if err != nil {
		a.stationError(w, r, err)
		return
	}
	a.writeJSON(w, r, page)
}

// queryValues rejects unknown, duplicated, and malformed query parameters.
func queryValues(r *http.Request, allowed ...string) (url.Values, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, invalid("invalid query: " + err.Error())
	}
	for key, entries := range values {
		if !slices.Contains(allowed, key) || len(entries) != 1 {
			return nil, invalid("unknown or duplicated query parameter: " + key)
		}
	}
	return values, nil
}

func editIdentity(id, revision string) (string, uint64, error) {
	if strings.TrimSpace(id) == "" {
		return "", 0, invalid("simulationId is required")
	}
	n, err := decimalUint(revision)
	if err != nil || n == 0 {
		return "", 0, invalid("stationSetRevision must be a positive decimal uint64 string")
	}
	return id, n, nil
}

// decimalUint accepts a canonical unsigned decimal representation, preserving
// uint64 identities without float conversion or JavaScript precision loss.
func decimalUint(s string) (uint64, error) {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, err
	}
	if strconv.FormatUint(n, 10) != s {
		return 0, fmt.Errorf("noncanonical unsigned integer %q", s)
	}
	return n, nil
}

func invalid(message string) error { return fmt.Errorf("%w: %s", simulation.ErrInvalid, message) }

func (a *api) stationError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	problem := simulatorapi.APIError{Error: err.Error()}
	switch {
	case errors.Is(err, simulatorapi.ErrUnavailable):
		status = http.StatusServiceUnavailable
	case errors.Is(err, simulation.ErrInvalid), errors.Is(err, simulatorapi.ErrInvalidRequest):
		status = http.StatusBadRequest
	case errors.Is(err, simulation.ErrNotFound), errors.Is(err, simulatorapi.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, simulation.ErrConflict), errors.Is(err, simulatorapi.ErrConflict):
		status = http.StatusConflict
		set := a.sim.Stations()
		problem.SimulationID, problem.StationSetRevision = set.SimulationID, &set.Revision
	default:
		a.logger.ErrorContext(r.Context(), "station API request", "path", r.URL.Path, "error", err)
		problem.Error = "could not apply station request"
	}
	a.writeStatusJSON(w, r, status, problem)
}

// readStation bounds the entire body, including trailing whitespace, then
// checks required values before converting the wire definition to engine input.
func (a *api) readStation(w http.ResponseWriter, r *http.Request) (simulatorapi.StationRequest, uint64, simulation.StationDefinition, bool) {
	var input simulatorapi.StationRequest
	var definition simulation.StationDefinition
	if _, err := queryValues(r); err != nil {
		a.stationError(w, r, err)
		return input, 0, definition, false
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		a.writeStatusJSON(w, r, http.StatusUnsupportedMediaType, simulatorapi.APIError{Error: "Content-Type must be application/json"})
		return input, 0, definition, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, simulatorapi.StationRequestLimit))
	if err != nil {
		var tooLarge *http.MaxBytesError
		status := http.StatusBadRequest
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		a.writeStatusJSON(w, r, status, simulatorapi.APIError{Error: "read station request: " + err.Error()})
		return input, 0, definition, false
	}
	if err := strictDecode(body, &input); err != nil {
		a.stationError(w, r, invalid(err.Error()))
		return input, 0, definition, false
	}
	_, revision, err := editIdentity(input.SimulationID, input.StationSetRevision)
	if err != nil {
		a.stationError(w, r, err)
		return input, 0, definition, false
	}
	definition, fields := parseDefinition(input.Definition)
	if len(fields) > 0 {
		a.writeStatusJSON(w, r, http.StatusBadRequest, simulatorapi.APIError{Error: "invalid station definition", Fields: fields})
		return input, 0, definition, false
	}
	return input, revision, definition, true
}

func strictDecode(body []byte, value any) error {
	if len(bytes.TrimSpace(body)) == 0 || bytes.TrimSpace(body)[0] != '{' {
		return errors.New("request must contain one JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON object")
	}
	return nil
}

func parseDefinition(raw json.RawMessage) (simulation.StationDefinition, map[string]string) {
	fields := make(map[string]string)
	object := requiredFields(raw, "", fields, "name", "latitude", "longitude", "enabled", "antennaHeightMeters", "receiveGainDbi", "feederLossDb", "channelA", "channelB", "shadowSectors")
	for _, channel := range []string{"channelA", "channelB"} {
		requiredFields(object[channel], channel, fields, "enabled", "sensitivityDbm", "noisePenaltyDb", "dropProbability")
	}
	var sectors []json.RawMessage
	if err := json.Unmarshal(object["shadowSectors"], &sectors); err != nil {
		fields["shadowSectors"] = "must be an array"
	}
	for i, sector := range sectors {
		requiredFields(sector, fmt.Sprintf("shadowSectors.%d", i), fields, "startDegrees", "endDegrees", "lossDb")
	}
	var wire simulatorapi.StationDefinition
	if err := strictDecode(raw, &wire); err != nil {
		fields["definition"] = err.Error()
	}
	d := simulation.StationDefinition{
		Name: wire.Name, Latitude: wire.Latitude, Longitude: wire.Longitude, Enabled: wire.Enabled,
		AntennaHeightMeters: wire.AntennaHeightMeters, ReceiveGainDBi: wire.ReceiveGainDBi, FeederLossDB: wire.FeederLossDB,
		ChannelA: simulation.ReceiverChannel(wire.ChannelA), ChannelB: simulation.ReceiverChannel(wire.ChannelB),
		ShadowSectors: make([]simulation.ShadowSector, 0, len(wire.ShadowSectors)),
	}
	for _, s := range wire.ShadowSectors {
		d.ShadowSectors = append(d.ShadowSectors, simulation.ShadowSector(s))
	}
	if len(fields) == 0 {
		if err := simulation.ValidateStation(d); err != nil {
			stationValidationFields(err, "", fields)
		}
		encoded, err := json.Marshal(wire)
		if err != nil {
			fields["definition"] = err.Error()
		} else if len(encoded) > simulatorapi.StationDefinitionLimit {
			fields["definition"] = "encoded definition exceeds 4 KiB"
		}
	}
	return d, fields
}

// stationValidationFields preserves individual engine diagnostics from joined
// validation errors and associates them with the form field or sector group.
// Channel context lives on the wrapping error, not on each range diagnostic.
func stationValidationFields(err error, prefix string, fields map[string]string) {
	if err == simulation.ErrInvalid {
		return
	}
	for _, channel := range []string{"A", "B"} {
		if strings.HasPrefix(err.Error(), "channel "+channel+":") {
			prefix = "channel" + channel + "."
		}
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			stationValidationFields(child, prefix, fields)
		}
		return
	}
	if child := errors.Unwrap(err); child != nil {
		stationValidationFields(child, prefix, fields)
		return
	}
	key := "definition"
	for _, field := range [][2]string{
		{"name", "name"}, {"latitude", "latitude"}, {"longitude", "longitude"},
		{"antenna height", "antennaHeightMeters"}, {"receive gain", "receiveGainDbi"}, {"feeder loss", "feederLossDb"},
		{"sensitivity", "sensitivityDbm"}, {"noise penalty", "noisePenaltyDb"}, {"drop probability", "dropProbability"},
	} {
		if strings.HasPrefix(err.Error(), field[0]) {
			key = prefix + field[1]
			break
		}
	}
	if strings.Contains(err.Error(), "shadow sector") {
		key = "shadowSectors"
	}
	if fields[key] != "" {
		fields[key] += "\n"
	}
	fields[key] += err.Error()
}

// requiredFields distinguishes a missing/null input from valid zero or false.
func requiredFields(raw json.RawMessage, prefix string, errorsByField map[string]string, names ...string) map[string]json.RawMessage {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		if prefix == "" {
			prefix = "definition"
		}
		errorsByField[prefix] = "must be an object"
	}
	for _, name := range names {
		value, exists := object[name]
		if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			key := name
			if prefix != "" {
				key = prefix + "." + name
			}
			errorsByField[key] = "is required"
		}
	}
	return object
}
