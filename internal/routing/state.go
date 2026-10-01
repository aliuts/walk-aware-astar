package routing

import "walking-aware-nav/internal/model"

// StateKey distinguishes locations reached with different walking progress.
// Progress is represented in integer microseconds and capped at the requested
// minimum, so satisfying the lower bound does not create unbounded states.
type StateKey struct {
	Node         model.NodeID
	WalkingUnits int64
	LastService  string
	OnTransit    bool
}

// SearchCost contains the values used to compare two labels with the same
// state key.
type SearchCost struct {
	TravelTime   float64
	WalkingUnits int64
}

type stateGroup struct {
	Node        model.NodeID
	LastService string
	OnTransit   bool
}

func groupFor(state StateKey) stateGroup {
	return stateGroup{Node: state.Node, LastService: state.LastService, OnTransit: state.OnTransit}
}
