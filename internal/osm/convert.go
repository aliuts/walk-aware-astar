package osm

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"walking-aware-nav/internal/geo"
	"walking-aware-nav/internal/model"
)

const (
	DefaultWalkingSpeedMetersPerSecond = 1.4
	namedPointConnectionDistance       = 250.0
)

type namedPointConnection struct {
	PointID     int64
	WalkingNode int64
	Distance    float64
}

// BuildWalkingNetwork converts pedestrian-accessible OSM ways into the
// application's graph format. OSM node IDs are retained as graph IDs.
//
// OSM geometry nodes are usually anonymous. Names are therefore resolved in
// this order: a node's own name, its containing named way, and finally a
// generic internal label. Named standalone points are connected to the
// nearest walking node so they can be selected as route endpoints.
func BuildWalkingNetwork(response Response, walkingSpeedMetersPerSecond float64) (model.NetworkFile, error) {
	if walkingSpeedMetersPerSecond <= 0 || math.IsNaN(walkingSpeedMetersPerSecond) || math.IsInf(walkingSpeedMetersPerSecond, 0) {
		return model.NetworkFile{}, fmt.Errorf("walking speed must be finite and positive")
	}

	nodesByID := make(map[int64]Element)
	ways := make([]Element, 0)
	for _, element := range response.Elements {
		switch element.Type {
		case "node":
			if finiteCoordinate(element.Lat, element.Lon) {
				previous, exists := nodesByID[element.ID]
				if exists {
					if element.Tags == nil {
						element.Tags = make(map[string]string)
					}
					for key, value := range previous.Tags {
						if _, hasValue := element.Tags[key]; !hasValue {
							element.Tags[key] = value
						}
					}
				}
				nodesByID[element.ID] = element
			}
		case "way":
			if PedestrianAllowed(element.Tags) && len(element.Nodes) > 1 {
				ways = append(ways, element)
			}
		}
	}
	if len(ways) == 0 {
		return model.NetworkFile{}, fmt.Errorf("OSM response contains no usable pedestrian ways")
	}
	sort.Slice(ways, func(i, j int) bool { return ways[i].ID < ways[j].ID })

	wayNodeIDs := make(map[int64]struct{})
	wayNodeNames := make(map[int64]string)
	for _, way := range ways {
		wayName := strings.TrimSpace(way.Tags["name"])
		if wayName == "" {
			wayName = strings.TrimSpace(way.Tags["ref"])
		}
		for _, nodeID := range way.Nodes {
			if _, exists := nodesByID[nodeID]; !exists {
				continue
			}
			wayNodeIDs[nodeID] = struct{}{}
			if wayName != "" && wayNodeNames[nodeID] == "" {
				wayNodeNames[nodeID] = wayName
			}
		}
	}
	if len(wayNodeIDs) == 0 {
		return model.NetworkFile{}, fmt.Errorf("OSM ways reference no usable nodes")
	}

	wayNodeIDList := sortedNodeIDs(wayNodeIDs)
	namedConnections := make([]namedPointConnection, 0)
	for nodeID, element := range nodesByID {
		if _, isWayNode := wayNodeIDs[nodeID]; isWayNode || !hasUsefulName(element.Tags) {
			continue
		}
		nearestID, distance := nearestWayNode(element, nodesByID, wayNodeIDList)
		if distance <= namedPointConnectionDistance {
			namedConnections = append(namedConnections, namedPointConnection{
				PointID:     nodeID,
				WalkingNode: nearestID,
				Distance:    distance,
			})
		}
	}
	sort.Slice(namedConnections, func(i, j int) bool { return namedConnections[i].PointID < namedConnections[j].PointID })

	graphNodeIDs := make(map[int64]struct{}, len(wayNodeIDs)+len(namedConnections))
	for nodeID := range wayNodeIDs {
		graphNodeIDs[nodeID] = struct{}{}
	}
	for _, connection := range namedConnections {
		graphNodeIDs[connection.PointID] = struct{}{}
	}
	nodeIDs := sortedNodeIDs(graphNodeIDs)

	network := model.NetworkFile{
		Nodes: make([]model.Node, 0, len(nodeIDs)),
		Edges: make([]model.Edge, 0),
	}
	for _, nodeID := range nodeIDs {
		element := nodesByID[nodeID]
		name, named := nodeName(element, wayNodeNames[nodeID], nodeID)
		network.Nodes = append(network.Nodes, model.Node{
			ID:        model.NodeID(nodeID),
			Name:      name,
			Named:     named,
			Latitude:  element.Lat,
			Longitude: element.Lon,
		})
	}

	for _, way := range ways {
		orderedNodes := way.Nodes
		if onewayReverse(way.Tags) {
			orderedNodes = reverseCopy(orderedNodes)
		}
		bidirectional := !isOneWay(way.Tags)
		for index := 0; index < len(orderedNodes)-1; index++ {
			from, fromOK := nodesByID[orderedNodes[index]]
			to, toOK := nodesByID[orderedNodes[index+1]]
			if !fromOK || !toOK {
				continue
			}
			distance := geo.DistanceMeters(from.Lat, from.Lon, to.Lat, to.Lon)
			if distance <= 0 {
				continue
			}
			duration := distance / walkingSpeedMetersPerSecond / 60
			network.Edges = append(network.Edges, walkingEdge(from.ID, to.ID, duration, distance))
			if bidirectional {
				network.Edges = append(network.Edges, walkingEdge(to.ID, from.ID, duration, distance))
			}
		}
	}

	for _, connection := range namedConnections {
		distance := connection.Distance
		if distance == 0 {
			distance = 1
		}
		duration := distance / walkingSpeedMetersPerSecond / 60
		network.Edges = append(network.Edges,
			walkingEdge(connection.PointID, connection.WalkingNode, duration, distance),
			walkingEdge(connection.WalkingNode, connection.PointID, duration, distance),
		)
	}

	if len(network.Edges) == 0 {
		return model.NetworkFile{}, fmt.Errorf("OSM response produced no walking edges")
	}
	return network, nil
}

func walkingEdge(from, to int64, duration, distance float64) model.Edge {
	return model.Edge{
		From:            model.NodeID(from),
		To:              model.NodeID(to),
		Mode:            model.ModeWalk,
		DurationMinutes: duration,
		DistanceMeters:  distance,
	}
}

func sortedNodeIDs(nodes map[int64]struct{}) []int64 {
	result := make([]int64, 0, len(nodes))
	for nodeID := range nodes {
		result = append(result, nodeID)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func nearestWayNode(point Element, nodes map[int64]Element, wayNodeIDs []int64) (int64, float64) {
	bestID := int64(0)
	bestDistance := math.Inf(1)
	for _, nodeID := range wayNodeIDs {
		node := nodes[nodeID]
		distance := geo.DistanceMeters(point.Lat, point.Lon, node.Lat, node.Lon)
		if distance < bestDistance {
			bestID = nodeID
			bestDistance = distance
		}
	}
	return bestID, bestDistance
}

func hasUsefulName(tags map[string]string) bool {
	return strings.TrimSpace(tags["name"]) != ""
}

func nodeName(element Element, wayName string, nodeID int64) (string, bool) {
	if name := strings.TrimSpace(element.Tags["name"]); name != "" {
		return name, true
	}
	if wayName != "" {
		return wayName, true
	}
	return fmt.Sprintf("Walking node %d", nodeID), false
}

// PedestrianAllowed applies the importer's intentionally small OSM tag policy.
func PedestrianAllowed(tags map[string]string) bool {
	if tags == nil {
		return false
	}
	if tags["foot"] == "no" || tags["foot"] == "private" || tags["access"] == "no" || tags["access"] == "private" {
		return false
	}

	switch tags["highway"] {
	case "footway", "pedestrian", "path", "track", "steps", "living_street", "residential", "service", "unclassified", "tertiary", "secondary", "primary", "cycleway":
		return true
	default:
		return false
	}
}

func isOneWay(tags map[string]string) bool {
	switch tags["oneway"] {
	case "yes", "true", "1", "-1":
		return true
	default:
		return false
	}
}

func onewayReverse(tags map[string]string) bool {
	return tags["oneway"] == "-1"
}

func reverseCopy(values []int64) []int64 {
	result := make([]int64, len(values))
	for index, value := range values {
		result[len(values)-1-index] = value
	}
	return result
}

func finiteCoordinate(latitude, longitude float64) bool {
	return !math.IsNaN(latitude) && !math.IsInf(latitude, 0) && !math.IsNaN(longitude) && !math.IsInf(longitude, 0) && latitude >= -90 && latitude <= 90 && longitude >= -180 && longitude <= 180
}
