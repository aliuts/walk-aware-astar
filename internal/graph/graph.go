package graph

import (
	"fmt"
	"math"

	"walking-aware-nav/internal/model"
)

const defaultMaxSpeedMetersPerMinute = 1000.0

// Graph is an adjacency-list representation of the network.
type Graph struct {
	Nodes map[model.NodeID]model.Node
	Edges map[model.NodeID][]model.Edge

	// MaxSpeedMetersPerMinute is used to keep the geographic heuristic
	// admissible for the edges present in this graph.
	MaxSpeedMetersPerMinute float64
}

// New validates and builds a graph from its nodes and edges.
func New(nodes []model.Node, edges []model.Edge) (*Graph, error) {
	g := &Graph{
		Nodes: make(map[model.NodeID]model.Node, len(nodes)),
		Edges: make(map[model.NodeID][]model.Edge, len(nodes)),
	}

	for _, node := range nodes {
		if node.Name == "" {
			return nil, fmt.Errorf("node %d has an empty name", node.ID)
		}
		if !finite(node.Latitude) || !finite(node.Longitude) {
			return nil, fmt.Errorf("node %d has non-finite coordinates", node.ID)
		}
		if node.Latitude < -90 || node.Latitude > 90 {
			return nil, fmt.Errorf("node %d has invalid latitude", node.ID)
		}
		if node.Longitude < -180 || node.Longitude > 180 {
			return nil, fmt.Errorf("node %d has invalid longitude", node.ID)
		}
		if _, exists := g.Nodes[node.ID]; exists {
			return nil, fmt.Errorf("duplicate node id %d", node.ID)
		}

		g.Nodes[node.ID] = node
		g.Edges[node.ID] = nil
	}

	maxSpeed := defaultMaxSpeedMetersPerMinute
	for _, edge := range edges {
		if _, exists := g.Nodes[edge.From]; !exists {
			return nil, fmt.Errorf("edge references unknown from node %d", edge.From)
		}
		if _, exists := g.Nodes[edge.To]; !exists {
			return nil, fmt.Errorf("edge references unknown to node %d", edge.To)
		}
		if !validMode(edge.Mode) {
			return nil, fmt.Errorf("edge %d -> %d has unsupported mode %q", edge.From, edge.To, edge.Mode)
		}
		if !finite(edge.DurationMinutes) || edge.DurationMinutes <= 0 {
			return nil, fmt.Errorf("edge %d -> %d has invalid duration", edge.From, edge.To)
		}
		if !finite(edge.DistanceMeters) || edge.DistanceMeters < 0 {
			return nil, fmt.Errorf("edge %d -> %d has invalid distance", edge.From, edge.To)
		}

		g.Edges[edge.From] = append(g.Edges[edge.From], edge)
		if edge.DistanceMeters > 0 {
			speed := edge.DistanceMeters / edge.DurationMinutes
			if speed > maxSpeed {
				maxSpeed = speed
			}
		}
	}

	// Walking is undirected in the demo network. Add a reverse edge only when
	// the input does not already provide one explicitly.
	for _, edge := range edges {
		if edge.Mode != model.ModeWalk || hasReverseWalk(g.Edges[edge.To], edge) {
			continue
		}
		reverse := edge
		reverse.From, reverse.To = edge.To, edge.From
		g.Edges[reverse.From] = append(g.Edges[reverse.From], reverse)
	}

	g.MaxSpeedMetersPerMinute = maxSpeed
	return g, nil
}

func validMode(mode model.TransportMode) bool {
	switch mode {
	case model.ModeWalk, model.ModeBus, model.ModeTrain:
		return true
	default:
		return false
	}
}

func hasReverseWalk(edges []model.Edge, edge model.Edge) bool {
	for _, candidate := range edges {
		if candidate.To == edge.From && candidate.Mode == model.ModeWalk {
			return true
		}
	}
	return false
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
