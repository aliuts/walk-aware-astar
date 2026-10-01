package gtfs

import (
	"fmt"
	"math"
	"sort"

	"walking-aware-nav/internal/geo"
	"walking-aware-nav/internal/model"
)

const (
	DefaultWalkingSpeedMetersPerSecond = 1.4
	DefaultMaxTransferDistanceMeters   = 500.0
)

// MergeOptions controls how transit stops are joined to the walking graph.
type MergeOptions struct {
	Bounds                      geo.Bounds
	FilterStopsToBounds         bool
	WalkingSpeedMetersPerSecond float64
	MaxTransferDistanceMeters   float64
}

type transitEdge struct {
	FromStop string
	ToStop   string
	Edge     model.Edge
}

// MergeWithWalking adds GTFS stop nodes, static transit edges, and walking
// connections from each nearby stop to the existing OSM graph.
func MergeWithWalking(network model.NetworkFile, feed Feed, options MergeOptions) (model.NetworkFile, error) {
	speed := options.WalkingSpeedMetersPerSecond
	if speed <= 0 || math.IsNaN(speed) || math.IsInf(speed, 0) {
		speed = DefaultWalkingSpeedMetersPerSecond
	}
	maxTransferDistance := options.MaxTransferDistanceMeters
	if maxTransferDistance <= 0 || math.IsNaN(maxTransferDistance) || math.IsInf(maxTransferDistance, 0) {
		maxTransferDistance = DefaultMaxTransferDistanceMeters
	}
	if len(network.Nodes) == 0 {
		return model.NetworkFile{}, fmt.Errorf("cannot merge GTFS into an empty walking graph")
	}

	stopIDs := make([]string, 0, len(feed.Stops))
	for stopID, stop := range feed.Stops {
		if options.FilterStopsToBounds && !options.Bounds.Contains(stop.Latitude, stop.Longitude) {
			continue
		}
		stopIDs = append(stopIDs, stopID)
	}
	sort.Strings(stopIDs)
	if len(stopIDs) == 0 {
		return model.NetworkFile{}, fmt.Errorf("GTFS contains no stops inside the import bounds")
	}

	result := model.NetworkFile{
		Nodes: append([]model.Node(nil), network.Nodes...),
		Edges: append([]model.Edge(nil), network.Edges...),
	}
	usedIDs := make(map[model.NodeID]struct{}, len(result.Nodes))
	for _, node := range result.Nodes {
		usedIDs[node.ID] = struct{}{}
	}

	stopNodeIDs := make(map[string]model.NodeID, len(stopIDs))
	selectedStopIDs := make(map[string]struct{}, len(stopIDs))
	nextID := model.NodeID(-1)
	for _, stopID := range stopIDs {
		for {
			if _, used := usedIDs[nextID]; !used {
				break
			}
			nextID--
		}
		stop := feed.Stops[stopID]
		stopNodeIDs[stopID] = nextID
		selectedStopIDs[stopID] = struct{}{}
		usedIDs[nextID] = struct{}{}
		result.Nodes = append(result.Nodes, model.Node{
			ID:        nextID,
			Name:      stop.Name,
			Named:     true,
			Latitude:  stop.Latitude,
			Longitude: stop.Longitude,
		})
		nextID--
	}

	walkingNodes := append([]model.Node(nil), network.Nodes...)
	for _, stopID := range stopIDs {
		stop := feed.Stops[stopID]
		nearest, distance := nearestNode(stop, walkingNodes)
		if distance > maxTransferDistance {
			continue
		}
		accessDistance := distance
		if accessDistance == 0 {
			// Keep coincident stop/street nodes connected without generating a
			// zero-duration edge, which the runtime graph rejects.
			accessDistance = 1
		}
		duration := accessDistance / speed / 60
		stopNodeID := stopNodeIDs[stopID]
		result.Edges = append(result.Edges,
			model.Edge{From: stopNodeID, To: nearest.ID, Mode: model.ModeWalk, DurationMinutes: duration, DistanceMeters: accessDistance},
			model.Edge{From: nearest.ID, To: stopNodeID, Mode: model.ModeWalk, DurationMinutes: duration, DistanceMeters: accessDistance},
		)
	}

	transitEdges := buildTransitEdges(feed, selectedStopIDs)
	for _, candidate := range transitEdges {
		from, fromOK := stopNodeIDs[candidate.FromStop]
		to, toOK := stopNodeIDs[candidate.ToStop]
		if !fromOK || !toOK {
			continue
		}
		candidate.Edge.From = from
		candidate.Edge.To = to
		result.Edges = append(result.Edges, candidate.Edge)
	}
	if len(transitEdges) == 0 {
		return model.NetworkFile{}, fmt.Errorf("GTFS contains no supported static transit connections")
	}
	return result, nil
}

func nearestNode(stop Stop, nodes []model.Node) (model.Node, float64) {
	bestDistance := math.Inf(1)
	var nearest model.Node
	for _, node := range nodes {
		distance := geo.DistanceMeters(stop.Latitude, stop.Longitude, node.Latitude, node.Longitude)
		if distance < bestDistance {
			bestDistance = distance
			nearest = node
		}
	}
	return nearest, bestDistance
}

func buildTransitEdges(feed Feed, selectedStopIDs map[string]struct{}) []transitEdge {
	byTrip := make(map[string][]StopTime)
	for _, stopTime := range feed.StopTimes {
		byTrip[stopTime.TripID] = append(byTrip[stopTime.TripID], stopTime)
	}

	type edgeKey struct {
		from    string
		to      string
		mode    model.TransportMode
		service string
	}
	best := make(map[edgeKey]transitEdge)
	tripIDs := make([]string, 0, len(byTrip))
	for tripID := range byTrip {
		tripIDs = append(tripIDs, tripID)
	}
	sort.Strings(tripIDs)

	for _, tripID := range tripIDs {
		trip, tripOK := feed.Trips[tripID]
		if !tripOK {
			continue
		}
		route, routeOK := feed.Routes[trip.RouteID]
		if !routeOK {
			continue
		}
		mode, supported := modeForRouteType(route.Type)
		if !supported {
			continue
		}
		service := route.ShortName
		if service == "" {
			service = route.ID
		}

		times := byTrip[tripID]
		sort.Slice(times, func(i, j int) bool { return times[i].Sequence < times[j].Sequence })
		for index := 0; index < len(times)-1; index++ {
			fromTime := times[index]
			toTime := times[index+1]
			fromStop, fromOK := feed.Stops[fromTime.StopID]
			toStop, toOK := feed.Stops[toTime.StopID]
			_, fromSelected := selectedStopIDs[fromTime.StopID]
			_, toSelected := selectedStopIDs[toTime.StopID]
			if !fromOK || !toOK || !fromSelected || !toSelected || fromTime.StopID == toTime.StopID {
				continue
			}
			durationSeconds := toTime.ArrivalSeconds - fromTime.DepartureSeconds
			if durationSeconds <= 0 {
				continue
			}
			key := edgeKey{from: fromStop.ID, to: toStop.ID, mode: mode, service: service}
			candidate := transitEdge{
				FromStop: fromStop.ID,
				ToStop:   toStop.ID,
				Edge: model.Edge{
					Mode:            mode,
					DurationMinutes: float64(durationSeconds) / 60,
					DistanceMeters:  geo.DistanceMeters(fromStop.Latitude, fromStop.Longitude, toStop.Latitude, toStop.Longitude),
					Service:         service,
				},
			}
			previous, exists := best[key]
			if !exists || candidate.Edge.DurationMinutes < previous.Edge.DurationMinutes {
				best[key] = candidate
			}
		}
	}

	keys := make([]edgeKey, 0, len(best))
	for key := range best {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].from != keys[j].from {
			return keys[i].from < keys[j].from
		}
		if keys[i].to != keys[j].to {
			return keys[i].to < keys[j].to
		}
		if keys[i].mode != keys[j].mode {
			return keys[i].mode < keys[j].mode
		}
		return keys[i].service < keys[j].service
	})

	result := make([]transitEdge, 0, len(keys))
	for _, key := range keys {
		result = append(result, best[key])
	}
	return result
}

func modeForRouteType(routeType int) (model.TransportMode, bool) {
	switch routeType {
	case 0, 1, 2, 12:
		return model.ModeTrain, true
	case 3, 11:
		return model.ModeBus, true
	}
	// GTFS route_type extensions use 100-series values for rail and
	// 700-series values for bus services.
	if routeType >= 100 && routeType <= 199 {
		return model.ModeTrain, true
	}
	if routeType >= 700 && routeType <= 799 {
		return model.ModeBus, true
	}
	return "", false
}
