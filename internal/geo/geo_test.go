package geo

import "testing"

func TestParseBounds(t *testing.T) {
	bounds, err := ParseBounds("-33.9,151.1,-33.8,151.3")
	if err != nil {
		t.Fatalf("ParseBounds returned error: %v", err)
	}
	if !bounds.Contains(-33.85, 151.2) || bounds.Contains(-34, 151.2) {
		t.Fatalf("bounds containment is incorrect: %+v", bounds)
	}
}

func TestDistanceMeters(t *testing.T) {
	distance := DistanceMeters(0, 0, 0, 1)
	if distance < 111_000 || distance > 112_000 {
		t.Fatalf("distance = %f, want approximately 111,195 metres", distance)
	}
}
