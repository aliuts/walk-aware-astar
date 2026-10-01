package routing

import (
	"container/heap"
	"math"
	"sort"

	"walking-aware-nav/internal/graph"
	"walking-aware-nav/internal/model"
)

const (
	waypointFallbackNodeThreshold = 5000
	maxWaypointCandidates         = 1024
)

type waypointCandidate struct {
	node     model.NodeID
	prefix   bool
	estimate float64
}

type pathLink struct {
	edge model.Edge
}

type weightedPathResult struct {
	distance map[model.NodeID]float64
	previous map[model.NodeID]pathLink
	explored int
}

type weightedQueueItem struct {
	node  model.NodeID
	cost  float64
	index int
}

type weightedQueue []*weightedQueueItem

func (queue weightedQueue) Len() int {
	return len(queue)
}

func (queue weightedQueue) Less(i, j int) bool {
	if queue[i].cost != queue[j].cost {
		return queue[i].cost < queue[j].cost
	}
	return queue[i].node < queue[j].node
}

func (queue weightedQueue) Swap(i, j int) {
	queue[i], queue[j] = queue[j], queue[i]
	queue[i].index = i
	queue[j].index = j
}

func (queue *weightedQueue) Push(value any) {
	item := value.(*weightedQueueItem)
	item.index = len(*queue)
	*queue = append(*queue, item)
}

func (queue *weightedQueue) Pop() any {
	old := *queue
	last := len(old) - 1
	item := old[last]
	old[last] = nil
	*queue = old[:last]
	item.index = -1
	return item
}

func findRouteWithWalkingWaypoint(network *graph.Graph, request model.RouteRequest) (*model.Route, error) {
	requiredMinutes := 0.0
	if request.MinWalkingMinutes != nil {
		requiredMinutes = *request.MinWalkingMinutes
	}
	walkingFromStart := shortestPaths(network, request.From, true, false)
	walkingToGoal := shortestPaths(network, request.To, true, true)
	fastestFromStart := shortestPaths(network, request.From, false, false)
	fastestToGoal := shortestPaths(network, request.To, false, true)

	candidates := make([]waypointCandidate, 0)
	for node := range network.Nodes {
		if distance, ok := walkingFromStart.distance[node]; ok && distance+comparisonEpsilon >= requiredMinutes {
			if suffix, exists := fastestToGoal.distance[node]; exists {
				candidates = append(candidates, waypointCandidate{
					node:     node,
					prefix:   true,
					estimate: distance + suffix,
				})
			}
		}
		if distance, ok := walkingToGoal.distance[node]; ok && distance+comparisonEpsilon >= requiredMinutes {
			if prefix, exists := fastestFromStart.distance[node]; exists {
				candidates = append(candidates, waypointCandidate{
					node:     node,
					prefix:   false,
					estimate: prefix + distance,
				})
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].estimate < candidates[j].estimate
	})
	if len(candidates) > maxWaypointCandidates {
		candidates = candidates[:maxWaypointCandidates]
	}

	var best *model.Route
	var shortTransitBest *model.Route
	explored := walkingFromStart.explored + walkingToGoal.explored + fastestFromStart.explored + fastestToGoal.explored
	considerRoute := func(route model.Route, edges []model.Edge) {
		if routeHasRepeatedNode(edges) {
			return
		}
		route.NodesExplored = explored
		if routeHasShortTransitRun(route) {
			if shortTransitBest == nil || betterWaypointRoute(route, *shortTransitBest) {
				shortTransitBest = &route
			}
			return
		}
		if best == nil || betterWaypointRoute(route, *best) {
			best = &route
		}
	}
	for _, candidate := range candidates {
		var edges []model.Edge
		if candidate.prefix {
			prefix, ok := pathEdges(walkingFromStart, request.From, candidate.node, false)
			if !ok {
				continue
			}
			suffix, ok := pathEdges(fastestToGoal, request.To, candidate.node, true)
			if !ok {
				continue
			}
			edges = append(prefix, suffix...)
		} else {
			prefix, ok := pathEdges(fastestFromStart, request.From, candidate.node, false)
			if !ok {
				continue
			}
			suffix, ok := pathEdges(walkingToGoal, request.To, candidate.node, true)
			if !ok {
				continue
			}
			edges = append(prefix, suffix...)
		}

		edges = repairServiceReentries(network, edges)
		route := routeFromEdges(network, edges)
		if route.WalkingMinutes+comparisonEpsilon < requiredMinutes {
			continue
		}
		considerRoute(route, edges)
	}
	if best == nil {
		best = shortTransitBest
	}
	if best == nil {
		return nil, ErrNoRoute
	}
	return best, nil
}

func shortestPaths(network *graph.Graph, source model.NodeID, walkingOnly, reverse bool) weightedPathResult {
	distance := make(map[model.NodeID]float64, len(network.Nodes))
	previous := make(map[model.NodeID]pathLink, len(network.Nodes))
	incoming := make(map[model.NodeID][]model.Edge)
	if reverse {
		for _, edges := range network.Edges {
			for _, edge := range edges {
				if isExcludedTransitEdge(edge) {
					continue
				}
				if !walkingOnly || edge.Mode == model.ModeWalk {
					incoming[edge.To] = append(incoming[edge.To], edge)
				}
			}
		}
	}

	queue := &weightedQueue{}
	heap.Init(queue)
	distance[source] = 0
	heap.Push(queue, &weightedQueueItem{node: source})
	explored := 0

	for queue.Len() > 0 {
		item := heap.Pop(queue).(*weightedQueueItem)
		best, ok := distance[item.node]
		if !ok || math.Abs(item.cost-best) > comparisonEpsilon {
			continue
		}
		explored++

		if reverse {
			for _, edge := range incoming[item.node] {
				considerWeightedPath(queue, distance, previous, item.node, edge.From, edge, item.cost)
			}
			continue
		}
		for _, edge := range network.Edges[item.node] {
			if isExcludedTransitEdge(edge) {
				continue
			}
			if walkingOnly && edge.Mode != model.ModeWalk {
				continue
			}
			considerWeightedPath(queue, distance, previous, item.node, edge.To, edge, item.cost)
		}
	}

	return weightedPathResult{distance: distance, previous: previous, explored: explored}
}

func considerWeightedPath(queue *weightedQueue, distance map[model.NodeID]float64, previous map[model.NodeID]pathLink, from, to model.NodeID, edge model.Edge, currentCost float64) {
	candidate := currentCost + edge.DurationMinutes
	old, exists := distance[to]
	if exists && candidate >= old-comparisonEpsilon {
		return
	}
	distance[to] = candidate
	previous[to] = pathLink{edge: edge}
	heap.Push(queue, &weightedQueueItem{node: to, cost: candidate})
}

func repairServiceReentries(network *graph.Graph, edges []model.Edge) []model.Edge {
	repaired := append([]model.Edge(nil), edges...)
	for {
		changed := false
		for index := 0; index < len(repaired); index++ {
			if repaired[index].Mode == model.ModeWalk {
				continue
			}
			service := serviceKey(repaired[index])
			switchIndex := index + 1
			for switchIndex < len(repaired) && repaired[switchIndex].Mode != model.ModeWalk && serviceKey(repaired[switchIndex]) == service {
				switchIndex++
			}
			if switchIndex >= len(repaired) || repaired[switchIndex].Mode == model.ModeWalk {
				continue
			}

			reentryIndex := switchIndex + 1
			for reentryIndex < len(repaired) && repaired[reentryIndex].Mode != model.ModeWalk && serviceKey(repaired[reentryIndex]) != service {
				reentryIndex++
			}
			if reentryIndex >= len(repaired) || repaired[reentryIndex].Mode == model.ModeWalk {
				continue
			}

			replacement, ok := shortestServicePath(network, repaired[switchIndex].From, repaired[reentryIndex].From, service)
			if !ok || edgeDuration(replacement) > edgeDuration(repaired[switchIndex:reentryIndex])+transitTransferPenaltyMinutes+comparisonEpsilon {
				continue
			}

			updated := make([]model.Edge, 0, len(repaired)-reentryIndex+switchIndex+len(replacement))
			updated = append(updated, repaired[:switchIndex]...)
			updated = append(updated, replacement...)
			updated = append(updated, repaired[reentryIndex:]...)
			repaired = updated
			changed = true
			break
		}
		if !changed {
			return repaired
		}
	}
}

func shortestServicePath(network *graph.Graph, source, target model.NodeID, service string) ([]model.Edge, bool) {
	distance := make(map[model.NodeID]float64, len(network.Nodes))
	previous := make(map[model.NodeID]pathLink, len(network.Nodes))
	queue := &weightedQueue{}
	heap.Init(queue)
	distance[source] = 0
	heap.Push(queue, &weightedQueueItem{node: source})

	for queue.Len() > 0 {
		item := heap.Pop(queue).(*weightedQueueItem)
		best, ok := distance[item.node]
		if !ok || math.Abs(item.cost-best) > comparisonEpsilon {
			continue
		}
		if item.node == target {
			break
		}
		for _, edge := range network.Edges[item.node] {
			if isExcludedTransitEdge(edge) || edge.Mode == model.ModeWalk || serviceKey(edge) != service {
				continue
			}
			considerWeightedPath(queue, distance, previous, item.node, edge.To, edge, item.cost)
		}
	}

	return pathEdges(weightedPathResult{distance: distance, previous: previous}, source, target, false)
}

func edgeDuration(edges []model.Edge) float64 {
	total := 0.0
	for _, edge := range edges {
		total += edge.DurationMinutes
	}
	return total
}

func pathEdges(result weightedPathResult, source, target model.NodeID, reverse bool) ([]model.Edge, bool) {
	if source == target {
		return nil, true
	}
	edges := make([]model.Edge, 0)
	current := target
	for current != source {
		link, ok := result.previous[current]
		if !ok {
			return nil, false
		}
		edges = append(edges, link.edge)
		if reverse {
			current = link.edge.To
		} else {
			current = link.edge.From
		}
	}
	if !reverse {
		for left, right := 0, len(edges)-1; left < right; left, right = left+1, right-1 {
			edges[left], edges[right] = edges[right], edges[left]
		}
	}
	return edges, true
}

func betterWaypointRoute(candidate, best model.Route) bool {
	return routeOptimizationScore(candidate) < routeOptimizationScore(best)-comparisonEpsilon
}

func routeOptimizationScore(route model.Route) float64 {
	score := route.TotalMinutes
	onTransit := false
	lastService := ""
	for _, step := range route.Steps {
		isTransit := step.Mode != model.ModeWalk
		service := string(step.Mode) + ":" + step.Service
		if isTransit && !onTransit {
			if lastService == "" || lastService == service {
				score += transitBoardingPenaltyMinutes
			} else {
				score += transitTransferPenaltyMinutes
			}
		} else if isTransit && service != lastService {
			score += transitTransferPenaltyMinutes
		}
		onTransit = isTransit
		if isTransit {
			lastService = service
		}
	}
	return score
}

func routeHasRepeatedNode(edges []model.Edge) bool {
	if len(edges) == 0 {
		return false
	}
	seen := make(map[model.NodeID]struct{}, len(edges)+1)
	seen[edges[0].From] = struct{}{}
	for _, edge := range edges {
		if _, exists := seen[edge.To]; exists {
			return true
		}
		seen[edge.To] = struct{}{}
	}
	return false
}

func routeHasShortTransitRun(route model.Route) bool {
	currentService := ""
	currentDuration := 0.0
	flush := func() bool {
		return currentDuration > 0 && currentDuration < minimumPreferredTransitRunMinutes
	}

	for _, step := range route.Steps {
		if step.Mode == model.ModeWalk {
			if flush() {
				return true
			}
			currentService = ""
			currentDuration = 0
			continue
		}
		service := string(step.Mode) + ":" + step.Service
		if service != currentService {
			if flush() {
				return true
			}
			currentService = service
			currentDuration = 0
		}
		currentDuration += step.DurationMinutes
	}
	return flush()
}
