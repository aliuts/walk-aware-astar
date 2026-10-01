package osm

import (
	"strings"
	"testing"

	"walking-aware-nav/internal/model"
)

func TestDecodeAndBuildWalkingNetwork(t *testing.T) {
	response, err := Decode(strings.NewReader(`{
  "elements": [
    {"type":"node","id":1,"lat":0,"lon":0,"tags":{"name":"Sydney Central"}},
    {"type":"node","id":2,"lat":0,"lon":0.001},
    {"type":"node","id":3,"lat":0,"lon":0.002},
    {"type":"node","id":4,"lat":0,"lon":0.003},
    {"type":"way","id":10,"nodes":[1,2,3],"tags":{"highway":"footway"}},
    {"type":"way","id":11,"nodes":[3,4],"tags":{"highway":"residential","name":"George Street","oneway":"yes"}},
    {"type":"way","id":12,"nodes":[4,1],"tags":{"highway":"motorway"}}
  ]
}`))
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}

	network, err := BuildWalkingNetwork(response, 1.4)
	if err != nil {
		t.Fatalf("BuildWalkingNetwork returned error: %v", err)
	}
	if len(network.Nodes) != 4 {
		t.Fatalf("node count = %d, want 4", len(network.Nodes))
	}
	if len(network.Edges) != 5 {
		t.Fatalf("edge count = %d, want 5", len(network.Edges))
	}

	for _, edge := range network.Edges {
		if edge.Mode != model.ModeWalk || edge.DurationMinutes <= 0 || edge.DistanceMeters <= 0 {
			t.Fatalf("invalid walking edge: %+v", edge)
		}
	}
	if network.Nodes[0].Name != "Sydney Central" || !network.Nodes[0].Named {
		t.Fatalf("named OSM node = %+v", network.Nodes[0])
	}
	if network.Nodes[2].Name != "George Street" || !network.Nodes[2].Named {
		t.Fatalf("named way node = %+v", network.Nodes[2])
	}
	if PedestrianAllowed(map[string]string{"highway": "motorway"}) {
		t.Fatal("motorway was treated as pedestrian-accessible")
	}
	if PedestrianAllowed(map[string]string{"highway": "footway", "access": "private"}) {
		t.Fatal("private footway was treated as pedestrian-accessible")
	}
}

func TestBuildWalkingNetworkHonoursReverseOneway(t *testing.T) {
	response := Response{Elements: []Element{
		{Type: "node", ID: 1, Lat: 0, Lon: 0},
		{Type: "node", ID: 2, Lat: 0, Lon: 0.001},
		{Type: "way", ID: 1, Nodes: []int64{1, 2}, Tags: map[string]string{"highway": "service", "oneway": "-1"}},
	}}

	network, err := BuildWalkingNetwork(response, 1.4)
	if err != nil {
		t.Fatalf("BuildWalkingNetwork returned error: %v", err)
	}
	if network.Edges[0].From != 2 || network.Edges[0].To != 1 {
		t.Fatalf("edge = %+v, want reverse direction", network.Edges[0])
	}
}
