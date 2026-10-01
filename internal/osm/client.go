package osm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"walking-aware-nav/internal/geo"
)

const defaultOverpassEndpoint = "https://overpass-api.de/api/interpreter"

// Client downloads a bounded OpenStreetMap extract from Overpass.
type Client struct {
	Endpoint   string
	HTTPClient *http.Client
	UserAgent  string
}

// Fetch executes a bounded pedestrian way query.
func (client Client) Fetch(ctx context.Context, bounds geo.Bounds) (Response, error) {
	endpoint := client.Endpoint
	if endpoint == "" {
		endpoint = defaultOverpassEndpoint
	}
	httpClient := client.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	form := url.Values{"data": {Query(bounds)}}.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form))
	if err != nil {
		return Response{}, fmt.Errorf("create Overpass request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if client.UserAgent != "" {
		request.Header.Set("User-Agent", client.UserAgent)
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return Response{}, fmt.Errorf("request Overpass: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return Response{}, fmt.Errorf("Overpass returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}

	return Decode(io.LimitReader(response.Body, 100<<20))
}

// Decode parses an Overpass JSON response from any reader.
func Decode(reader io.Reader) (Response, error) {
	var response Response
	if err := json.NewDecoder(reader).Decode(&response); err != nil {
		return Response{}, fmt.Errorf("decode OSM response: %w", err)
	}
	if len(response.Elements) == 0 {
		return Response{}, fmt.Errorf("OSM response contains no elements")
	}
	return response, nil
}

// Query returns the Overpass query used by the importer.
func Query(bounds geo.Bounds) string {
	return fmt.Sprintf(`[out:json][timeout:120];
(
  way["highway"](%g,%g,%g,%g);
  node["name"](%g,%g,%g,%g);
  node["place"](%g,%g,%g,%g);
  node["public_transport"](%g,%g,%g,%g);
  node["amenity"](%g,%g,%g,%g);
);
out body;
>;
out skel qt;`,
		bounds.South, bounds.West, bounds.North, bounds.East,
		bounds.South, bounds.West, bounds.North, bounds.East,
		bounds.South, bounds.West, bounds.North, bounds.East,
		bounds.South, bounds.West, bounds.North, bounds.East,
		bounds.South, bounds.West, bounds.North, bounds.East)
}
