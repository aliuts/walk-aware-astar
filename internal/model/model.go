package model

// NodeID identifies a location in the transport network. int64 preserves raw
// OpenStreetMap node IDs without requiring an importer-side remapping.
type NodeID int64

// Node is a location that can be reached by one or more edges.
type Node struct {
	ID        NodeID  `json:"id"`
	Name      string  `json:"name"`
	Named     bool    `json:"named,omitempty"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// TransportMode describes how an edge is traversed.
type TransportMode string

const (
	ModeWalk  TransportMode = "walk"
	ModeBus   TransportMode = "bus"
	ModeTrain TransportMode = "train"
)

// Edge connects two locations.
type Edge struct {
	From            NodeID        `json:"from"`
	To              NodeID        `json:"to"`
	Mode            TransportMode `json:"mode"`
	DurationMinutes float64       `json:"durationMinutes"`
	DistanceMeters  float64       `json:"distanceMeters"`
	Service         string        `json:"service,omitempty"`
}

// NetworkFile is the JSON representation used by the graph loader.
type NetworkFile struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// RouteRequest is the public routing query.
type RouteRequest struct {
	From              NodeID   `json:"from"`
	To                NodeID   `json:"to"`
	MinWalkingMinutes *float64 `json:"minWalkingMinutes,omitempty"`
}

// Route contains the selected path and its summary statistics.
type Route struct {
	Steps          []RouteStep `json:"steps"`
	TotalMinutes   float64     `json:"totalMinutes"`
	WalkingMinutes float64     `json:"walkingMinutes"`
	WalkingMeters  float64     `json:"walkingMeters"`
	Transfers      int         `json:"transfers"`
	NodesExplored  int         `json:"nodesExplored"`
}

// RouteStep is one traversed graph edge.
type RouteStep struct {
	From            Node          `json:"from"`
	To              Node          `json:"to"`
	Mode            TransportMode `json:"mode"`
	DurationMinutes float64       `json:"durationMinutes"`
	DistanceMeters  float64       `json:"distanceMeters"`
	Service         string        `json:"service,omitempty"`
}
