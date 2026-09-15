package simulator

import (
	"slices"

	"github.com/miroslav-matejovsky/ais-testbench/simulation"
	"github.com/miroslav-matejovsky/ais-testbench/simulatorapi"
)

// coverageGeometry cuts the engine's continuous-longitude ring into the three
// world strips it can intersect, then translates each piece into [-180,180].
// Engine station latitude is limited to +/-85, so no ring contains a pole.
func coverageGeometry(points []simulation.GeoPoint) simulatorapi.Geometry {
	g := simulatorapi.Geometry{Type: "MultiPolygon", Coordinates: make([][][][2]float64, 0)}
	if len(points) < 4 {
		return g
	}
	ring := make([][2]float64, 0, len(points)-1)
	for _, p := range points[:len(points)-1] {
		ring = append(ring, [2]float64{p.Longitude, p.Latitude})
	}
	for strip := -1; strip <= 1; strip++ {
		offset := float64(strip) * 360
		piece := clipLongitude(clipLongitude(ring, offset-180, true), offset+180, false)
		if len(piece) < 3 {
			continue
		}
		for i := range piece {
			piece[i][0] -= offset
		}
		// Shoelace area sets GeoJSON's exterior winding and rejects zero-area pieces.
		area := 0.0
		for i, p := range piece {
			q := piece[(i+1)%len(piece)]
			area += p[0]*q[1] - q[0]*p[1]
		}
		if area == 0 {
			continue
		}
		if area < 0 {
			slices.Reverse(piece)
		}
		piece = append(piece, piece[0])
		g.Coordinates = append(g.Coordinates, [][][2]float64{piece})
	}
	return g
}

// clipLongitude clips an open ring against one vertical half-plane. Crossing
// points interpolate latitude on the original sampled edge, preserving contours.
func clipLongitude(ring [][2]float64, boundary float64, keepGreater bool) [][2]float64 {
	out := make([][2]float64, 0, len(ring)+2)
	if len(ring) == 0 {
		return out
	}
	inside := func(p [2]float64) bool {
		if keepGreater {
			return p[0] >= boundary
		}
		return p[0] <= boundary
	}
	previous := ring[len(ring)-1]
	for _, current := range ring {
		if inside(previous) != inside(current) {
			t := (boundary - previous[0]) / (current[0] - previous[0])
			out = append(out, [2]float64{boundary, previous[1] + t*(current[1]-previous[1])})
		}
		if inside(current) {
			out = append(out, current)
		}
		previous = current
	}
	return out
}
