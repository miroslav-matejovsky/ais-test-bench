---
title: "02 - Deterministic reception and coverage model"
dependencies: ["01-station-configuration.md"]
effort: "L"
complexity: "high"
---

# Deterministic reception and coverage model

## Outcome

Calculate a station's receiving probability and estimated signal power for each
transmission, and derive displayable coverage from exactly that calculation.
The [research](research.md) records the evidence and the model's limitations.

## Implementation work

### Channels and transmission metadata

Extend `internal/ais` encoding to accept A or B explicitly, validate the value,
and checksum the final NMEA sentence. Extend decoding to return its channel.
One report is transmitted once on its selected channel, not once per receiver.
All receivers of that transmission retain the same complete NMEA bytes.

Alternate A/B using each vessel's report ordinal, with an initial phase derived
from stable vessel identity. A global sequence parity would leave some vessels
stuck on one channel with an even-sized fleet; do not use it. Stage the ordinal
with movement and encoding. Record channel and transmitter profile alongside the
transmission for reception evaluation. Do not put station ID or estimated power
inside the AIS payload or misuse the multipart sequence field for station identity.

### Pure link calculation

Use small concrete functions in `simulation`, with no clock, I/O, mutable RNG,
or display dependencies. Inputs are transmitter position/profile, receiver RF
configuration, and channel. Return distance, bearing, model horizon, estimated
receive power, effective sensitivity, margin, decode probability, and limiting
reason. Values are model diagnostics; display them with that qualification.

Use these initial model choices, versioned as `coastal-v1`:

```text
d_km = great_circle_distance_metres / 1000
d_eval = max(d_km, 0.01)
P_tx_dBm = 10 * log10(power_W * 1000)
H_km = 3.57 * sqrt(k) * (sqrt(tx_height_m) + sqrt(rx_height_m))
k = 4/3

L_free_dB = 32.4 + 20 * log10(channel_MHz) + 20 * log10(d_eval)
L_extra_dB = site_loss_dB + 10 * (path_exponent - 2) * log10(max(d_eval, 1))
P_rx_dBm = P_tx_dBm + tx_gain_dBi + rx_gain_dBi
           - tx_feeder_dB - rx_feeder_dB
           - L_free_dB - L_extra_dB - sector_loss_dB
S_effective_dBm = sensitivity_dBm + noise_penalty_dB
margin_dB = P_rx_dBm - S_effective_dBm
```

Use geodesic distance and bearing, not Web Mercator pixel distance. At coincident
positions report distance 0, use 10 m for the attenuation calculation, and apply
no directional sector because bearing is undefined. Represent that bearing as null.

Initial `site_loss_dB = 15` and `path_exponent = 3.5` are empirical scenario
settings, not values specified by the sources. Validate site loss in [0, 40] and
exponent in [2, 4]. Apply them consistently across all stations initially.
Changing these run-level parameters requires a new simulation run in the first
delivery. This avoids a second live configuration editor and invalidation path.

Define `p_margin` by piecewise linear interpolation through the knots
`(-12 dB, 0)`, `(0 dB, 0.8)`, and `(+6 dB, 1)`, clamped outside the knots.
This models 80% successful decoding at the sensitivity reference before other
impairments. Only that reference point is motivated by the standard; the curve
shape is a test-bench assumption.

Define `p_horizon = 1` through `0.8 * H`, decreasing linearly to 0 at `H`, and
0 beyond it. The taper is also a model choice. Then:

```text
p_receive = p_margin * p_horizon * (1 - extra_drop_probability)
```

Administrative disable or an unavailable channel forces `p_receive = 0`.
Do not double-count noise as both receive-power loss and sensitivity degradation.
Do not expose SNR unless a separate noise-power model is actually implemented.

Worked check for the reference transmitter and Rotterdam coast receiver, channel
A, no shadow: at 20 km, free-space loss is about 102.6 dB, extra loss about
34.5 dB, and estimated receive power about -94.2 dBm. The horizon is about
33.6 km. At 30 km, power is about -100.3 dBm and the horizon taper reduces
probability to about 0.54 even though margin remains positive. These rounded
values illustrate the proposed model; implementation tests use explicit tolerances.

### Reproducible receive/drop decisions

Derive `u` uniformly in [0, 1) from a stable, documented hash of a domain tag,
run seed, station ID, station RF revision, and transmission sequence. Encode
fields unambiguously, take 53 bits, and divide by `2^53`. Receive iff `u < p`.
Use an existing standard-library hash; avoid a new random framework. The run's
display identity is not part of the key, so changing only that identity does not
alter seeded RF behavior.

Never consume the vessel RNG for reception. Hashing each opportunity preserves
outcomes across batching, polling, station iteration order, and station additions.
For otherwise identical receivers, increasing sensitivity improves the probability;
realized packet subsets need not be nested after an RF revision changes the draw.

Record one deterministic outcome reason per evaluated station/report pair:
`station_disabled`, `channel_disabled`, `outside_horizon`, `insufficient_margin`,
`probabilistic_loss`, or `received`, in that precedence order. Zero probability
from an extra-drop setting is a probabilistic impairment, not an RF horizon loss.
Reasons describe the simulator's decision, not information decoded from the air.

### Coverage geometry

For each enabled station/channel, generate nested `p >= 0.90` and `p >= 0.50`
areas using the reference transmitter and the same pure probability function.
Sample every five degrees plus both sides of each shadow-sector boundary. Since
probability is non-increasing with distance along a fixed bearing, solve radius
with bounded binary search on [0, H]. Use fixed 16 iterations and a 256-vertex
cap per contour. If even the near-site probability is below a threshold, return
an empty contour rather than a fabricated radius.

Return plain WGS84 GeoJSON Polygon/MultiPolygon geometry from the simulator API,
with longitude/latitude ordering, closed rings, threshold, channel, RF revision,
model version, reference profile, and min/max radius. Handle antimeridian splits
and polar geometry or reject unsupported geometry explicitly during configuration
validation; do not silently draw a polygon across the world. Prefer global-safe
geometry since station coordinates are globally valid in step 01.

Cache immutable contour results by RF configuration within engine-owned station
state. No-op/name changes reuse them. Disabled channels return empty effective
coverage. Use ring outlines and a legend so overlapping polygons do not imply a
computed combined network probability. Polygon edges approximate the model;
actual decisions always evaluate the function, never point-in-polygon membership.

## Verification

- Table-driven numeric cases for unit conversion, loss/gain signs, reference
  power, distance clamp, true bearing, channel frequency, sector boundaries,
  wraparound, and horizon values.
- Probability knot/boundary cases; disabled sites/channels; identical input gives
  identical output; increased loss never improves probability at fixed geometry.
- A/B alternation at odd/even fleet sizes, creation reports, split advances,
  checksum validity, and exact sentence equality at multiple receivers.
- Fixed-seed hash vectors and a fixed population of marginal reports with a
  predeclared broad statistical tolerance. No nondeterministic sampling tests.
- Contour vertices match their probability threshold within solver tolerance;
  sampled interior/exterior checks allow five-degree angular approximation error.
  Include empty contours, deep sectors, antimeridian, and high-latitude cases.
- Golden numerical cases compare the proposed presets. Confirm sensitivity
  matters in impaired paths and antenna height matters near the horizon.

## Acceptance criteria

- Channel availability, sensitivity, gain, loss, height, and shadow sectors each
  have an observable and correctly directed effect on reception probability.
- Model assumptions and all units are public metadata, not implicit UI constants.
- Generated coverage uses the same RF function as reception, without browser RF logic.
- Repeatable reception does not alter existing movement or report scheduling.
