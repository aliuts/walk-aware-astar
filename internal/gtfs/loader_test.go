package gtfs

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"walking-aware-nav/internal/geo"
	"walking-aware-nav/internal/model"
)

func TestLoadZipAndMergeWithWalking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feed.zip")
	createTestFeed(t, path)

	feed, err := LoadZip(path)
	if err != nil {
		t.Fatalf("LoadZip returned error: %v", err)
	}
	if len(feed.Stops) != 2 || len(feed.Routes) != 1 || len(feed.Trips) != 1 || len(feed.StopTimes) != 2 {
		t.Fatalf("feed counts are incorrect: %+v", feed)
	}

	network, err := MergeWithWalking(model.NetworkFile{
		Nodes: []model.Node{{ID: 100, Name: "Street", Latitude: 0, Longitude: 0}},
	}, feed, MergeOptions{
		Bounds:                      geo.Bounds{South: -1, West: -1, North: 1, East: 1},
		FilterStopsToBounds:         true,
		WalkingSpeedMetersPerSecond: 1.4,
		MaxTransferDistanceMeters:   500,
	})
	if err != nil {
		t.Fatalf("MergeWithWalking returned error: %v", err)
	}
	if len(network.Nodes) != 3 {
		t.Fatalf("merged node count = %d, want 3", len(network.Nodes))
	}
	if len(network.Edges) != 5 {
		t.Fatalf("merged edge count = %d, want 5", len(network.Edges))
	}

	var foundTransit bool
	for _, edge := range network.Edges {
		if edge.Mode == model.ModeTrain {
			foundTransit = true
			if edge.DurationMinutes != 5 || edge.Service != "T1" {
				t.Fatalf("transit edge = %+v", edge)
			}
		}
	}
	if !foundTransit {
		t.Fatal("merged network has no transit edge")
	}
}

func TestParseGTFSClockSupportsAfterMidnightValues(t *testing.T) {
	seconds, err := ParseGTFSClock("25:10:05")
	if err != nil {
		t.Fatalf("ParseGTFSClock returned error: %v", err)
	}
	if seconds != 90_605 {
		t.Fatalf("seconds = %d, want 90605", seconds)
	}
}

func TestModeForExtendedRouteTypes(t *testing.T) {
	if mode, ok := modeForRouteType(700); !ok || mode != model.ModeBus {
		t.Fatalf("700 route type = %q, %v; want bus, true", mode, ok)
	}
	if mode, ok := modeForRouteType(109); !ok || mode != model.ModeTrain {
		t.Fatalf("109 route type = %q, %v; want train, true", mode, ok)
	}
}

func createTestFeed(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	files := map[string]string{
		"stops.txt":      "stop_id,stop_name,stop_lat,stop_lon\nS1,Central,0,0.001\nS2,Town Hall,0,0.002\n",
		"routes.txt":     "route_id,route_short_name,route_type\nR1,T1,2\n",
		"trips.txt":      "route_id,service_id,trip_id\nR1,WK,T1-trip\n",
		"stop_times.txt": "trip_id,arrival_time,departure_time,stop_id,stop_sequence\nT1-trip,08:00:00,08:00:00,S1,1\nT1-trip,08:05:00,08:05:00,S2,2\n",
	}
	for name, contents := range files {
		writer, createErr := archive.Create(name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, writeErr := writer.Write([]byte(contents)); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
