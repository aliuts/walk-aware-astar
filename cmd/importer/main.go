package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"walking-aware-nav/internal/geo"
	"walking-aware-nav/internal/gtfs"
	"walking-aware-nav/internal/model"
	"walking-aware-nav/internal/osm"
)

func main() {
	boundsValue := flag.String("bbox", "-33.895,151.190,-33.865,151.220", "south,west,north,east import bounds")
	osmFile := flag.String("osm-file", "", "local Overpass JSON file; downloads from Overpass when empty")
	osmTileSize := flag.Float64("osm-tile-size", 0, "split OSM downloads into overlapping tiles of this degree size; 0 disables tiling")
	overpassURL := flag.String("overpass-url", "", "Overpass interpreter URL")
	gtfsFile := flag.String("gtfs", "", "optional local GTFS ZIP file")
	gtfsURL := flag.String("gtfs-url", "", "optional GTFS ZIP URL; downloaded to -gtfs or data/raw/gtfs.zip")
	gtfsAPIKey := flag.String("gtfs-api-key", "", "optional TfNSW API key for a GTFS URL")
	output := flag.String("output", "data/network.json", "output network JSON path")
	walkingSpeed := flag.Float64("walking-speed", osm.DefaultWalkingSpeedMetersPerSecond, "walking speed in metres per second")
	maxTransferDistance := flag.Float64("max-transfer-distance", gtfs.DefaultMaxTransferDistanceMeters, "maximum OSM-to-GTFS connection distance in metres")
	userAgent := flag.String("user-agent", "walking-aware-nav/1.0", "User-Agent sent to Overpass")
	timeout := flag.Duration("timeout", 30*time.Minute, "overall import deadline")
	flag.Parse()

	bounds, err := geo.ParseBounds(*boundsValue)
	if err != nil {
		log.Fatalf("invalid bbox: %v", err)
	}
	if *walkingSpeed <= 0 {
		log.Fatal("walking-speed must be positive")
	}
	if math.IsNaN(*osmTileSize) || math.IsInf(*osmTileSize, 0) || *osmTileSize < 0 {
		log.Fatal("osm-tile-size must be finite and non-negative")
	}
	if *gtfsFile != "" && *gtfsURL != "" {
		log.Fatal("use either -gtfs or -gtfs-url, not both")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	response, err := loadOSM(ctx, *osmFile, *overpassURL, *userAgent, bounds, *osmTileSize)
	if err != nil {
		log.Fatal(err)
	}
	network, err := osm.BuildWalkingNetwork(response, *walkingSpeed)
	if err != nil {
		log.Fatalf("build walking network: %v", err)
	}

	feedPath := *gtfsFile
	if *gtfsURL != "" {
		if feedPath == "" {
			feedPath = "data/raw/gtfs.zip"
		}
		if err := downloadGTFS(ctx, *gtfsURL, *gtfsAPIKey, *userAgent, feedPath); err != nil {
			log.Fatal(err)
		}
	}
	if feedPath != "" {
		feed, loadErr := gtfs.LoadZip(feedPath)
		if loadErr != nil {
			log.Fatal(loadErr)
		}
		network, err = gtfs.MergeWithWalking(network, feed, gtfs.MergeOptions{
			Bounds:                      bounds,
			FilterStopsToBounds:         true,
			WalkingSpeedMetersPerSecond: *walkingSpeed,
			MaxTransferDistanceMeters:   *maxTransferDistance,
		})
		if err != nil {
			log.Fatalf("merge GTFS network: %v", err)
		}
	}

	if err := writeNetwork(*output, network); err != nil {
		log.Fatal(err)
	}
	busEdges, trainEdges := transitEdgeCounts(network)
	log.Printf("wrote %d nodes and %d edges (%d bus, %d train) to %s", len(network.Nodes), len(network.Edges), busEdges, trainEdges, *output)
}

func loadOSM(ctx context.Context, path, endpoint, userAgent string, bounds geo.Bounds, tileSize float64) (osm.Response, error) {
	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return osm.Response{}, fmt.Errorf("open OSM file: %w", err)
		}
		defer file.Close()
		return osm.Decode(file)
	}

	client := osm.Client{Endpoint: endpoint, UserAgent: userAgent}
	if tileSize <= 0 {
		return client.Fetch(ctx, bounds)
	}

	tiles := splitBounds(bounds, tileSize)
	responses := make([]osm.Response, 0, len(tiles))
	for index, tile := range tiles {
		log.Printf("downloading OSM tile %d/%d", index+1, len(tiles))
		response, err := client.Fetch(ctx, tile)
		if err != nil {
			return osm.Response{}, fmt.Errorf("download OSM tile %d/%d: %w", index+1, len(tiles), err)
		}
		responses = append(responses, response)
	}
	return osm.MergeResponses(responses...), nil
}

func splitBounds(bounds geo.Bounds, tileSize float64) []geo.Bounds {
	const overlap = 0.001
	const splitEpsilon = 1e-9
	rowCount := int(math.Ceil((bounds.North-bounds.South)/tileSize - splitEpsilon))
	columnCount := int(math.Ceil((bounds.East-bounds.West)/tileSize - splitEpsilon))
	tiles := make([]geo.Bounds, 0, rowCount*columnCount)
	for row := 0; row < rowCount; row++ {
		south := bounds.South + float64(row)*tileSize
		north := south + tileSize
		if row == rowCount-1 || north > bounds.North {
			north = bounds.North
		}
		for column := 0; column < columnCount; column++ {
			west := bounds.West + float64(column)*tileSize
			east := west + tileSize
			if column == columnCount-1 || east > bounds.East {
				east = bounds.East
			}
			tiles = append(tiles, geo.Bounds{
				South: math.Max(bounds.South, south-overlap),
				West:  math.Max(bounds.West, west-overlap),
				North: math.Min(bounds.North, north+overlap),
				East:  math.Min(bounds.East, east+overlap),
			})
		}
	}
	return tiles
}

func downloadGTFS(ctx context.Context, endpoint, apiKey, userAgent, path string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create GTFS request: %w", err)
	}
	if userAgent != "" {
		request.Header.Set("User-Agent", userAgent)
	}
	if apiKey != "" {
		request.Header.Set("Authorization", "apikey "+apiKey)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("download GTFS: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("download GTFS: server returned %s", response.Status)
	}

	if directory := filepath.Dir(path); directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create GTFS directory: %w", err)
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create GTFS file: %w", err)
	}
	if _, err := io.Copy(file, response.Body); err != nil {
		file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write GTFS file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close GTFS file: %w", err)
	}
	log.Printf("downloaded GTFS feed to %s", path)
	return nil
}

func writeNetwork(path string, network model.NetworkFile) error {
	if directory := filepath.Dir(path); directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create output network: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(network); err != nil {
		return fmt.Errorf("write output network: %w", err)
	}
	return nil
}

func transitEdgeCounts(network model.NetworkFile) (bus, train int) {
	for _, edge := range network.Edges {
		switch edge.Mode {
		case model.ModeBus:
			bus++
		case model.ModeTrain:
			train++
		}
	}
	return bus, train
}
