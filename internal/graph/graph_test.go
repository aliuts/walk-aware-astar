package graph

import (
	"os"
	"path/filepath"
	"testing"

	"walking-aware-nav/internal/model"
)

func TestNewAddsReverseWalkingEdges(t *testing.T) {
	network, err := New(
		[]model.Node{
			{ID: 1, Name: "A", Latitude: 0, Longitude: 0},
			{ID: 2, Name: "B", Latitude: 0, Longitude: 0.001},
		},
		[]model.Edge{{From: 1, To: 2, Mode: model.ModeWalk, DurationMinutes: 2, DistanceMeters: 150}},
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if got := len(network.Edges[2]); got != 1 {
		t.Fatalf("reverse edge count = %d, want 1", got)
	}
	reverse := network.Edges[2][0]
	if reverse.To != 1 || reverse.Mode != model.ModeWalk {
		t.Fatalf("reverse edge = %+v", reverse)
	}
}

func TestNewRejectsMalformedEdges(t *testing.T) {
	_, err := New(
		[]model.Node{{ID: 1, Name: "A", Latitude: 0, Longitude: 0}},
		[]model.Edge{{From: 1, To: 99, Mode: model.ModeBus, DurationMinutes: 2}},
	)
	if err == nil {
		t.Fatal("New accepted an unknown edge endpoint")
	}
}

func TestLoadGraphReadsJSON(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "network.json")
	contents := `{
  "nodes": [
    {"id": 1, "name": "A", "latitude": 0, "longitude": 0},
    {"id": 2, "name": "B", "latitude": 0, "longitude": 0.001}
  ],
  "edges": [
    {"from": 1, "to": 2, "mode": "bus", "durationMinutes": 3, "distanceMeters": 100, "service": "1"}
  ]
}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	network, err := LoadGraph(path)
	if err != nil {
		t.Fatalf("LoadGraph returned error: %v", err)
	}
	if len(network.Nodes) != 2 || len(network.Edges[1]) != 1 {
		t.Fatalf("loaded graph = %+v", network)
	}
}
