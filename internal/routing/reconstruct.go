package routing

import (
	"math"

	"walking-aware-nav/internal/graph"
	"walking-aware-nav/internal/model"
)

// ReconstructRoute follows the queue item's immutable predecessor chain and
// computes the user-facing route statistics.
func ReconstructRoute(network *graph.Graph, goal *QueueItem) model.Route {
	reversed := make([]model.Edge, 0)
	current := goal

	for current.Parent != nil {
		previous := current.Parent
		reversed = append(reversed, current.Edge)
		current = previous
	}

	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	reversed = repairServiceReentries(network, reversed)
	return routeFromEdges(network, reversed)
}

func routeFromEdges(network *graph.Graph, edges []model.Edge) model.Route {
	steps := make([]model.RouteStep, 0, len(edges))
	for _, edge := range edges {
		steps = append(steps, model.RouteStep{
			From:            network.Nodes[edge.From],
			To:              network.Nodes[edge.To],
			Mode:            edge.Mode,
			DurationMinutes: edge.DurationMinutes,
			DistanceMeters:  edge.DistanceMeters,
			Service:         edge.Service,
		})
	}

	route := model.Route{Steps: steps}
	lastTransitService := ""
	for _, step := range steps {
		route.TotalMinutes += step.DurationMinutes
		if step.Mode == model.ModeWalk {
			route.WalkingMinutes += step.DurationMinutes
			route.WalkingMeters += step.DistanceMeters
			continue
		}

		serviceKey := string(step.Mode) + ":" + step.Service
		if lastTransitService != "" && serviceKey != lastTransitService {
			route.Transfers++
		}
		lastTransitService = serviceKey
	}

	route.TotalMinutes = round(route.TotalMinutes)
	route.WalkingMinutes = round(route.WalkingMinutes)
	route.WalkingMeters = round(route.WalkingMeters)
	return route
}

func round(value float64) float64 {
	return math.Round(value*100) / 100
}
