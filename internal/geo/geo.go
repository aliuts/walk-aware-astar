package geo

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const earthRadiusMeters = 6_371_000.0

// Bounds describes a south, west, north, east geographic rectangle.
type Bounds struct {
	South float64
	West  float64
	North float64
	East  float64
}

// ParseBounds parses the Overpass convention: south,west,north,east.
func ParseBounds(value string) (Bounds, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 4 {
		return Bounds{}, fmt.Errorf("bounds must be south,west,north,east")
	}

	values := make([]float64, len(parts))
	for index, part := range parts {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return Bounds{}, fmt.Errorf("parse bounds value %q: %w", part, err)
		}
		values[index] = parsed
	}

	bounds := Bounds{South: values[0], West: values[1], North: values[2], East: values[3]}
	if !finite(bounds.South) || !finite(bounds.West) || !finite(bounds.North) || !finite(bounds.East) {
		return Bounds{}, fmt.Errorf("bounds must be finite")
	}
	if bounds.South < -90 || bounds.North > 90 || bounds.South >= bounds.North {
		return Bounds{}, fmt.Errorf("bounds have invalid latitude limits")
	}
	if bounds.West < -180 || bounds.East > 180 || bounds.West >= bounds.East {
		return Bounds{}, fmt.Errorf("bounds have invalid longitude limits")
	}
	return bounds, nil
}

// Contains reports whether a coordinate falls within the bounds.
func (bounds Bounds) Contains(latitude, longitude float64) bool {
	return latitude >= bounds.South && latitude <= bounds.North && longitude >= bounds.West && longitude <= bounds.East
}

// DistanceMeters returns the great-circle distance between two coordinates.
func DistanceMeters(latitude1, longitude1, latitude2, longitude2 float64) float64 {
	lat1 := latitude1 * math.Pi / 180
	lat2 := latitude2 * math.Pi / 180
	deltaLat := (latitude2 - latitude1) * math.Pi / 180
	deltaLon := (longitude2 - longitude1) * math.Pi / 180

	sineLat := math.Sin(deltaLat / 2)
	sineLon := math.Sin(deltaLon / 2)
	haversine := sineLat*sineLat + math.Cos(lat1)*math.Cos(lat2)*sineLon*sineLon

	return 2 * earthRadiusMeters * math.Asin(math.Sqrt(haversine))
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
