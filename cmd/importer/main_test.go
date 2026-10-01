package main

import (
	"testing"

	"walking-aware-nav/internal/geo"
)

func TestSplitBoundsCoversRequestedAreaWithOverlap(t *testing.T) {
	bounds := geo.Bounds{South: -34, West: 150, North: -33.9, East: 150.2}
	tiles := splitBounds(bounds, 0.05)
	if len(tiles) != 8 {
		t.Fatalf("tile count = %d, want 8", len(tiles))
	}
	if tiles[0].South != bounds.South || tiles[0].West != bounds.West {
		t.Fatalf("first tile starts at %+v, want south-west bounds", tiles[0])
	}
	last := tiles[len(tiles)-1]
	if last.North != bounds.North || last.East != bounds.East {
		t.Fatalf("last tile ends at %+v, want north-east bounds", last)
	}
}
