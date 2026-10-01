package osm

// Element is the subset of an Overpass element needed by the importer.
type Element struct {
	Type  string            `json:"type"`
	ID    int64             `json:"id"`
	Lat   float64           `json:"lat,omitempty"`
	Lon   float64           `json:"lon,omitempty"`
	Nodes []int64           `json:"nodes,omitempty"`
	Tags  map[string]string `json:"tags,omitempty"`
}

// Response is the JSON response returned by Overpass.
type Response struct {
	Elements []Element `json:"elements"`
}
