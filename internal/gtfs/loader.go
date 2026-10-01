package gtfs

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"
)

// LoadZip reads the standard GTFS files from a feed archive.
func LoadZip(path string) (Feed, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return Feed{}, fmt.Errorf("open GTFS archive: %w", err)
	}
	defer archive.Close()

	feed := Feed{
		Stops:  make(map[string]Stop),
		Routes: make(map[string]Route),
		Trips:  make(map[string]Trip),
	}

	rows, err := readRows(archive.File, "stops.txt", "stop_id", "stop_lat", "stop_lon")
	if err != nil {
		return Feed{}, err
	}
	for rowIndex, row := range rows {
		id := strings.TrimSpace(row["stop_id"])
		if id == "" {
			return Feed{}, fmt.Errorf("stops.txt row %d has an empty stop_id", rowIndex+2)
		}
		latitude, err := parseCoordinate(row["stop_lat"])
		if err != nil {
			return Feed{}, fmt.Errorf("stops.txt row %d latitude: %w", rowIndex+2, err)
		}
		longitude, err := parseCoordinate(row["stop_lon"])
		if err != nil {
			return Feed{}, fmt.Errorf("stops.txt row %d longitude: %w", rowIndex+2, err)
		}
		if latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
			return Feed{}, fmt.Errorf("stops.txt row %d has coordinates outside valid ranges", rowIndex+2)
		}
		name := strings.TrimSpace(row["stop_name"])
		if name == "" {
			name = id
		}
		feed.Stops[id] = Stop{ID: id, Name: name, Latitude: latitude, Longitude: longitude}
	}

	rows, err = readRows(archive.File, "routes.txt", "route_id", "route_type")
	if err != nil {
		return Feed{}, err
	}
	for rowIndex, row := range rows {
		id := strings.TrimSpace(row["route_id"])
		if id == "" {
			return Feed{}, fmt.Errorf("routes.txt row %d has an empty route_id", rowIndex+2)
		}
		routeType, err := strconv.Atoi(strings.TrimSpace(row["route_type"]))
		if err != nil {
			return Feed{}, fmt.Errorf("routes.txt row %d route_type: %w", rowIndex+2, err)
		}
		feed.Routes[id] = Route{ID: id, ShortName: strings.TrimSpace(row["route_short_name"]), Type: routeType}
	}

	rows, err = readRows(archive.File, "trips.txt", "route_id", "trip_id")
	if err != nil {
		return Feed{}, err
	}
	for rowIndex, row := range rows {
		tripID := strings.TrimSpace(row["trip_id"])
		routeID := strings.TrimSpace(row["route_id"])
		if tripID == "" || routeID == "" {
			return Feed{}, fmt.Errorf("trips.txt row %d has an empty trip or route id", rowIndex+2)
		}
		feed.Trips[tripID] = Trip{ID: tripID, RouteID: routeID}
	}

	rows, err = readRows(archive.File, "stop_times.txt", "trip_id", "arrival_time", "departure_time", "stop_id", "stop_sequence")
	if err != nil {
		return Feed{}, err
	}
	feed.StopTimes = make([]StopTime, 0, len(rows))
	for rowIndex, row := range rows {
		tripID := strings.TrimSpace(row["trip_id"])
		stopID := strings.TrimSpace(row["stop_id"])
		if tripID == "" || stopID == "" {
			return Feed{}, fmt.Errorf("stop_times.txt row %d has an empty trip or stop id", rowIndex+2)
		}
		arrival, err := parseClock(strings.TrimSpace(row["arrival_time"]))
		if err != nil {
			return Feed{}, fmt.Errorf("stop_times.txt row %d arrival_time: %w", rowIndex+2, err)
		}
		departure, err := parseClock(strings.TrimSpace(row["departure_time"]))
		if err != nil {
			return Feed{}, fmt.Errorf("stop_times.txt row %d departure_time: %w", rowIndex+2, err)
		}
		sequence, err := strconv.Atoi(strings.TrimSpace(row["stop_sequence"]))
		if err != nil {
			return Feed{}, fmt.Errorf("stop_times.txt row %d stop_sequence: %w", rowIndex+2, err)
		}
		feed.StopTimes = append(feed.StopTimes, StopTime{
			TripID:           tripID,
			StopID:           stopID,
			ArrivalSeconds:   arrival,
			DepartureSeconds: departure,
			Sequence:         sequence,
		})
	}

	if len(feed.Stops) == 0 || len(feed.Routes) == 0 || len(feed.Trips) == 0 || len(feed.StopTimes) == 0 {
		return Feed{}, fmt.Errorf("GTFS archive contains no usable stop, route, trip, or stop-time data")
	}
	return feed, nil
}

func readRows(files []*zip.File, filename string, required ...string) ([]map[string]string, error) {
	var archiveFile *zip.File
	for _, file := range files {
		if file.Name == filename || filepath.Base(file.Name) == filename {
			archiveFile = file
			break
		}
	}
	if archiveFile == nil {
		return nil, fmt.Errorf("GTFS archive is missing %s", filename)
	}

	reader, err := archiveFile.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filename, err)
	}
	defer reader.Close()

	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	csvReader.TrimLeadingSpace = true
	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("read %s header: %w", filename, err)
	}
	columns := make(map[string]int, len(header))
	for index, column := range header {
		columns[strings.TrimPrefix(strings.TrimSpace(column), "\ufeff")] = index
	}
	for _, column := range required {
		if _, ok := columns[column]; !ok {
			return nil, fmt.Errorf("%s is missing required column %s", filename, column)
		}
	}

	rows := make([]map[string]string, 0)
	for rowIndex := 2; ; rowIndex++ {
		values, readErr := csvReader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read %s row %d: %w", filename, rowIndex, readErr)
		}
		if len(values) != len(header) {
			return nil, fmt.Errorf("%s row %d has %d columns, want %d", filename, rowIndex, len(values), len(header))
		}
		row := make(map[string]string, len(header))
		for column, index := range columns {
			row[column] = values[index]
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// ParseGTFSClock converts HH:MM:SS, including GTFS hours above 23, to seconds.
func ParseGTFSClock(value string) (int, error) {
	return parseClock(value)
}

func parseClock(value string) (int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("clock value %q must be HH:MM:SS", value)
	}
	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid hours: %w", err)
	}
	minutes, err := strconv.Atoi(parts[1])
	if err != nil || minutes < 0 || minutes > 59 {
		return 0, fmt.Errorf("invalid minutes")
	}
	seconds, err := strconv.Atoi(parts[2])
	if err != nil || seconds < 0 || seconds > 59 {
		return 0, fmt.Errorf("invalid seconds")
	}
	if hours < 0 {
		return 0, fmt.Errorf("invalid hours")
	}
	return hours*3600 + minutes*60 + seconds, nil
}

func parseCoordinate(value string) (float64, error) {
	coordinate, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(coordinate) || math.IsInf(coordinate, 0) {
		return 0, fmt.Errorf("invalid coordinate %q", value)
	}
	return coordinate, nil
}
