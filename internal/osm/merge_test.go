package osm

import "testing"

func TestMergeResponsesDeduplicatesTileOverlap(t *testing.T) {
	merged := MergeResponses(
		Response{Elements: []Element{
			{Type: "node", ID: 1, Lat: -33.9, Lon: 151.1},
			{Type: "way", ID: 10, Nodes: []int64{1, 2}, Tags: map[string]string{"highway": "footway"}},
		}},
		Response{Elements: []Element{
			{Type: "node", ID: 1, Lat: -33.9, Lon: 151.1, Tags: map[string]string{"name": "Shared point"}},
			{Type: "node", ID: 2, Lat: -33.9, Lon: 151.101},
			{Type: "way", ID: 10, Nodes: []int64{1, 2}, Tags: map[string]string{"name": "Shared path"}},
		}},
	)

	if len(merged.Elements) != 3 {
		t.Fatalf("element count = %d, want 3", len(merged.Elements))
	}
	for _, element := range merged.Elements {
		if element.Type == "node" && element.ID == 1 {
			if element.Tags["name"] != "Shared point" {
				t.Fatalf("merged node tags = %+v", element.Tags)
			}
		}
		if element.Type == "way" && element.ID == 10 {
			if element.Tags["highway"] != "footway" || element.Tags["name"] != "Shared path" {
				t.Fatalf("merged way tags = %+v", element.Tags)
			}
		}
	}
}
