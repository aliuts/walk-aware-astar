package routing

import (
	"container/heap"
	"errors"
	"fmt"
	"math"

	"walking-aware-nav/internal/graph"
	"walking-aware-nav/internal/model"
)

var (
	ErrNoRoute        = errors.New("no route found")
	ErrInvalidRequest = errors.New("invalid route request")
)

const (
	secondsPerMinute                  = 60.0
	walkingUnitsPerSecond             = int64(1_000_000)
	walkingUnitsPerMinute             = int64(secondsPerMinute) * walkingUnitsPerSecond
	maxMinimumWalkingMinutesInput     = 7 * 24 * 60
	transitBoardingPenaltyMinutes     = 2.0
	transitTransferPenaltyMinutes     = 2.0
	minimumPreferredTransitRunMinutes = 2.0
	excludedTransitService            = "SHL"
	comparisonEpsilon                 = 1e-9
)

// FindRoute runs A* over (location, walking progress, transit service) states.
// The route must accumulate the requested minimum walking time while
// minimizing travel time.
func FindRoute(network *graph.Graph, request model.RouteRequest) (*model.Route, error) {
	if err := validateRequest(network, request); err != nil {
		return nil, err
	}

	minimumWalkingUnits := int64(0)
	if request.MinWalkingMinutes != nil {
		minimumWalkingUnits = walkingUnitsForMinimum(*request.MinWalkingMinutes)
	}
	start := StateKey{Node: request.From}
	if request.From == request.To {
		if minimumWalkingUnits > 0 {
			return nil, ErrNoRoute
		}
		return &model.Route{Steps: []model.RouteStep{}, NodesExplored: 1}, nil
	}
	if len(network.Nodes) > waypointFallbackNodeThreshold && request.MinWalkingMinutes != nil {
		baselineRequest := request
		baselineRequest.MinWalkingMinutes = nil
		baseline, baselineErr := FindRoute(network, baselineRequest)
		requiredMinutes := 0.0
		if request.MinWalkingMinutes != nil {
			requiredMinutes = *request.MinWalkingMinutes
		}
		if baselineErr == nil && baseline.WalkingMinutes+comparisonEpsilon >= requiredMinutes && !routeHasShortTransitRun(*baseline) {
			return baseline, nil
		}
		if baselineErr != nil && !errors.Is(baselineErr, ErrNoRoute) {
			return nil, baselineErr
		}
		return findRouteWithWalkingWaypoint(network, request)
	}

	queue := &PriorityQueue{}
	heap.Init(queue)
	startCost := SearchCost{}
	heap.Push(queue, makeQueueItem(network, start, startCost, request.To, minimumWalkingUnits))

	bestCost := map[StateKey]SearchCost{start: startCost}
	active := map[StateKey]bool{start: true}
	groups := map[stateGroup][]StateKey{
		groupFor(start): {start},
	}
	nodesExplored := 0

	for queue.Len() > 0 {
		item := heap.Pop(queue).(*QueueItem)
		if !active[item.State] {
			continue
		}
		best, exists := bestCost[item.State]
		if !exists || !costMatches(item.Cost, best) {
			continue
		}

		nodesExplored++
		if item.State.Node == request.To && item.Cost.WalkingUnits >= minimumWalkingUnits {
			route := ReconstructRoute(network, item)
			route.NodesExplored = nodesExplored
			return &route, nil
		}

		for _, edge := range network.Edges[item.State.Node] {
			if isExcludedTransitEdge(edge) || edge.To == item.State.Node || item.Parent != nil && edge.To == item.Parent.State.Node {
				continue
			}
			nextWalkingUnits := item.Cost.WalkingUnits
			if edge.Mode == model.ModeWalk {
				nextWalkingUnits += walkingUnitsForEdge(edge.DurationMinutes)
			}
			next := StateKey{
				Node:         edge.To,
				WalkingUnits: walkingProgress(nextWalkingUnits, minimumWalkingUnits),
				LastService:  item.State.LastService,
				OnTransit:    edge.Mode != model.ModeWalk,
			}
			nextCost := SearchCost{
				TravelTime:   item.Cost.TravelTime + edge.DurationMinutes,
				WalkingUnits: nextWalkingUnits,
			}
			if edge.Mode != model.ModeWalk {
				service := serviceKey(edge)
				next.LastService = service
				switch {
				case item.State.LastService == "":
					nextCost.TravelTime += transitBoardingPenaltyMinutes
				case item.State.LastService != service:
					nextCost.TravelTime += transitTransferPenaltyMinutes
				case !item.State.OnTransit:
					nextCost.TravelTime += transitBoardingPenaltyMinutes
				}
			}

			previousCost, seen := bestCost[next]
			if seen && !betterCost(nextCost, previousCost) {
				continue
			}

			if !addLabel(groups, active, bestCost, next, nextCost) {
				continue
			}
			nextItem := makeQueueItem(network, next, nextCost, request.To, minimumWalkingUnits)
			nextItem.Parent = item
			nextItem.Edge = edge
			heap.Push(queue, nextItem)
		}
	}

	return nil, ErrNoRoute
}

func validateRequest(network *graph.Graph, request model.RouteRequest) error {
	if network == nil {
		return fmt.Errorf("%w: graph is nil", ErrInvalidRequest)
	}
	if _, exists := network.Nodes[request.From]; !exists {
		return fmt.Errorf("%w: unknown origin %d", ErrInvalidRequest, request.From)
	}
	if _, exists := network.Nodes[request.To]; !exists {
		return fmt.Errorf("%w: unknown destination %d", ErrInvalidRequest, request.To)
	}
	if request.MinWalkingMinutes != nil {
		if math.IsNaN(*request.MinWalkingMinutes) || math.IsInf(*request.MinWalkingMinutes, 0) || *request.MinWalkingMinutes < 0 {
			return fmt.Errorf("%w: minWalkingMinutes must be finite and non-negative", ErrInvalidRequest)
		}
		if *request.MinWalkingMinutes > maxMinimumWalkingMinutesInput {
			return fmt.Errorf("%w: minWalkingMinutes is too large", ErrInvalidRequest)
		}
	}
	return nil
}

func walkingUnitsForEdge(minutes float64) int64 {
	scaled := minutes * secondsPerMinute * float64(walkingUnitsPerSecond)
	units := int64(math.Ceil(scaled))
	if units < 1 {
		return 1
	}
	return units
}

func walkingUnitsForMinimum(minutes float64) int64 {
	if minutes == 0 {
		return 0
	}
	return int64(math.Ceil(minutes * float64(walkingUnitsPerMinute)))
}

func walkingProgress(walkingUnits, minimumWalkingUnits int64) int64 {
	if walkingUnits >= minimumWalkingUnits {
		return minimumWalkingUnits
	}
	return walkingUnits
}

func betterCost(candidate, previous SearchCost) bool {
	return candidate.TravelTime < previous.TravelTime-comparisonEpsilon
}

func costMatches(candidate, best SearchCost) bool {
	return math.Abs(candidate.TravelTime-best.TravelTime) <= comparisonEpsilon
}

func addLabel(groups map[stateGroup][]StateKey, active map[StateKey]bool, bestCost map[StateKey]SearchCost, candidate StateKey, candidateCost SearchCost) bool {
	group := groupFor(candidate)
	labels := groups[group]
	survivors := make([]StateKey, 0, len(labels)+1)
	dominated := make([]StateKey, 0)
	for _, label := range labels {
		if !active[label] {
			continue
		}
		if label == candidate {
			dominated = append(dominated, label)
			continue
		}
		labelCost := bestCost[label]
		if dominates(labelCost, candidateCost) {
			return false
		}
		if dominates(candidateCost, labelCost) {
			dominated = append(dominated, label)
			continue
		}
		survivors = append(survivors, label)
	}

	for _, label := range dominated {
		active[label] = false
	}
	active[candidate] = true
	bestCost[candidate] = candidateCost
	groups[group] = append(survivors, candidate)
	return true
}

func dominates(first, second SearchCost) bool {
	if first.WalkingUnits < second.WalkingUnits {
		return false
	}

	firstNoWorse := first.TravelTime <= second.TravelTime+comparisonEpsilon
	strictlyBetter := first.WalkingUnits != second.WalkingUnits
	strictlyBetter = strictlyBetter || first.TravelTime < second.TravelTime-comparisonEpsilon
	return firstNoWorse && strictlyBetter
}

func makeQueueItem(network *graph.Graph, state StateKey, cost SearchCost, goal model.NodeID, minimumWalkingUnits int64) *QueueItem {
	item := &QueueItem{State: state, Cost: cost}
	remainingWalking := float64(minimumWalkingUnits-cost.WalkingUnits) / float64(walkingUnitsPerMinute)
	if remainingWalking < 0 {
		remainingWalking = 0
	}
	item.Primary = cost.TravelTime + math.Max(Heuristic(network, state.Node, goal), remainingWalking)
	item.Secondary = cost.TravelTime
	return item
}

func serviceKey(edge model.Edge) string {
	return string(edge.Mode) + ":" + edge.Service
}

func isExcludedTransitEdge(edge model.Edge) bool {
	return edge.Mode != model.ModeWalk && edge.Service == excludedTransitService
}
