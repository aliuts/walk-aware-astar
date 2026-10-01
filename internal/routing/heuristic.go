package routing

import (
	"walking-aware-nav/internal/geo"
	"walking-aware-nav/internal/graph"
	"walking-aware-nav/internal/model"
)

// DistanceBetween returns the great-circle distance between two coordinates.
func DistanceBetween(a, b model.Node) float64 {
	return geo.DistanceMeters(a.Latitude, a.Longitude, b.Latitude, b.Longitude)
}

// Heuristic estimates the remaining travel time in minutes. The graph's
// maximum observed edge speed keeps this a lower bound on any remaining path.
func Heuristic(network *graph.Graph, current, goal model.NodeID) float64 {
	if network == nil {
		return 0
	}
	currentNode, currentOK := network.Nodes[current]
	goalNode, goalOK := network.Nodes[goal]
	if !currentOK || !goalOK || network.MaxSpeedMetersPerMinute <= 0 {
		return 0
	}

	return DistanceBetween(currentNode, goalNode) / network.MaxSpeedMetersPerMinute
}
