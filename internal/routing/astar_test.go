package routing

import (
	"errors"
	"testing"

	"walking-aware-nav/internal/graph"
	"walking-aware-nav/internal/model"
)

func TestFindRouteRequiresMinimumWalking(t *testing.T) {
	network := mustGraph(t,
		[]model.Node{
			{ID: 1, Name: "Start", Latitude: 0, Longitude: 0},
			{ID: 2, Name: "Walk interchange", Latitude: 0, Longitude: 0.001},
			{ID: 3, Name: "Goal", Latitude: 0, Longitude: 0.002},
		},
		[]model.Edge{
			{From: 1, To: 3, Mode: model.ModeBus, DurationMinutes: 5, DistanceMeters: 1000, Service: "B1"},
			{From: 1, To: 2, Mode: model.ModeWalk, DurationMinutes: 8, DistanceMeters: 500},
			{From: 2, To: 3, Mode: model.ModeBus, DurationMinutes: 5, DistanceMeters: 1000, Service: "B1"},
		},
	)

	route, err := FindRoute(network, model.RouteRequest{From: 1, To: 3, MinWalkingMinutes: minutes(8)})
	if err != nil {
		t.Fatalf("FindRoute returned error: %v", err)
	}
	if route.TotalMinutes != 13 {
		t.Fatalf("total minutes = %v, want 13", route.TotalMinutes)
	}
	if route.WalkingMinutes != 8 {
		t.Fatalf("walking minutes = %v, want 8", route.WalkingMinutes)
	}
	if len(route.Steps) != 2 || route.Steps[0].Mode != model.ModeWalk {
		t.Fatalf("steps = %+v, want walk, bus", route.Steps)
	}
}

func TestFindRouteReturnsNoRouteBelowMinimumWalking(t *testing.T) {
	network := mustGraph(t,
		[]model.Node{
			{ID: 1, Name: "Start", Latitude: 0, Longitude: 0},
			{ID: 2, Name: "Goal", Latitude: 0, Longitude: 0.001},
		},
		[]model.Edge{{From: 1, To: 2, Mode: model.ModeWalk, DurationMinutes: 4, DistanceMeters: 300}},
	)

	_, err := FindRoute(network, model.RouteRequest{From: 1, To: 2, MinWalkingMinutes: minutes(5)})
	if !errors.Is(err, ErrNoRoute) {
		t.Fatalf("error = %v, want ErrNoRoute", err)
	}
}

func TestFindRouteDefaultsToFastest(t *testing.T) {
	network := mustGraph(t,
		[]model.Node{
			{ID: 1, Name: "Start", Latitude: 0, Longitude: 0},
			{ID: 2, Name: "Goal", Latitude: 0, Longitude: 0.001},
		},
		[]model.Edge{
			{From: 1, To: 2, Mode: model.ModeWalk, DurationMinutes: 1, DistanceMeters: 80},
			{From: 1, To: 2, Mode: model.ModeBus, DurationMinutes: 5, DistanceMeters: 1000, Service: "B1"},
		},
	)

	route, err := FindRoute(network, model.RouteRequest{From: 1, To: 2})
	if err != nil {
		t.Fatalf("FindRoute returned error: %v", err)
	}
	if route.TotalMinutes != 1 || route.WalkingMinutes != 1 {
		t.Fatalf("route = %+v, want the one-minute walking route", route)
	}
}

func TestFindRouteExcludesSHLService(t *testing.T) {
	network := mustGraph(t,
		[]model.Node{
			{ID: 1, Name: "Start", Latitude: 0, Longitude: 0},
			{ID: 2, Name: "Goal", Latitude: 0, Longitude: 0.001},
		},
		[]model.Edge{
			{From: 1, To: 2, Mode: model.ModeTrain, DurationMinutes: 1, DistanceMeters: 1000, Service: "SHL"},
			{From: 1, To: 2, Mode: model.ModeTrain, DurationMinutes: 3, DistanceMeters: 1000, Service: "T8"},
		},
	)

	route, err := FindRoute(network, model.RouteRequest{From: 1, To: 2})
	if err != nil {
		t.Fatalf("FindRoute returned error: %v", err)
	}
	if len(route.Steps) != 1 || route.Steps[0].Service == excludedTransitService {
		t.Fatalf("route = %+v, want a non-SHL service", route)
	}
}

func TestFindRouteLargeNetworkBaselineDoesNotRecurse(t *testing.T) {
	nodes := make([]model.Node, waypointFallbackNodeThreshold+1)
	for index := range nodes {
		nodes[index] = model.Node{ID: model.NodeID(index + 1), Name: "test node"}
	}
	network := mustGraph(t, nodes, []model.Edge{{
		From:            1,
		To:              2,
		Mode:            model.ModeWalk,
		DurationMinutes: 1,
		DistanceMeters:  80,
	}})

	route, err := FindRoute(network, model.RouteRequest{
		From:              1,
		To:                2,
		MinWalkingMinutes: minutes(1),
	})
	if err != nil {
		t.Fatalf("FindRoute returned error: %v", err)
	}
	if route.WalkingMinutes != 1 || route.TotalMinutes != 1 {
		t.Fatalf("route = %+v, want one minute of walking", route)
	}
}

func TestFindRouteAvoidsShortTransitBoarding(t *testing.T) {
	network := mustGraph(t,
		[]model.Node{
			{ID: 1, Name: "Start", Latitude: 0, Longitude: 0},
			{ID: 2, Name: "Goal", Latitude: 0, Longitude: 0.001},
		},
		[]model.Edge{
			{From: 1, To: 2, Mode: model.ModeWalk, DurationMinutes: 1.5, DistanceMeters: 120},
			{From: 1, To: 2, Mode: model.ModeBus, DurationMinutes: 1, DistanceMeters: 1000, Service: "B1"},
		},
	)

	route, err := FindRoute(network, model.RouteRequest{From: 1, To: 2})
	if err != nil {
		t.Fatalf("FindRoute returned error: %v", err)
	}
	if len(route.Steps) != 1 || route.Steps[0].Mode != model.ModeWalk {
		t.Fatalf("route = %+v, want the walking route instead of a one-minute bus ride", route)
	}
}

func TestFindRouteAvoidsUnnecessaryServiceChange(t *testing.T) {
	network := mustGraph(t,
		[]model.Node{
			{ID: 1, Name: "Start", Latitude: 0, Longitude: 0},
			{ID: 2, Name: "Interchange", Latitude: 0, Longitude: 0.001},
			{ID: 3, Name: "Second stop", Latitude: 0, Longitude: 0.002},
			{ID: 4, Name: "Goal", Latitude: 0, Longitude: 0.003},
		},
		[]model.Edge{
			{From: 1, To: 2, Mode: model.ModeWalk, DurationMinutes: 1, DistanceMeters: 80},
			{From: 2, To: 3, Mode: model.ModeBus, DurationMinutes: 2, DistanceMeters: 1000, Service: "B1"},
			{From: 3, To: 4, Mode: model.ModeBus, DurationMinutes: 1, DistanceMeters: 500, Service: "B2"},
			{From: 2, To: 4, Mode: model.ModeBus, DurationMinutes: 4, DistanceMeters: 1500, Service: "B1"},
		},
	)

	route, err := FindRoute(network, model.RouteRequest{From: 1, To: 4, MinWalkingMinutes: minutes(1)})
	if err != nil {
		t.Fatalf("FindRoute returned error: %v", err)
	}
	if route.Transfers != 0 || len(route.Steps) != 2 || route.Steps[1].Service != "B1" {
		t.Fatalf("route = %+v, want one continuous B1 service", route)
	}
}

func TestRepairServiceReentryKeepsTheOriginalService(t *testing.T) {
	network := mustGraph(t,
		[]model.Node{
			{ID: 1, Name: "Start", Latitude: 0, Longitude: 0},
			{ID: 2, Name: "First stop", Latitude: 0, Longitude: 0.001},
			{ID: 3, Name: "Second stop", Latitude: 0, Longitude: 0.002},
			{ID: 4, Name: "Goal", Latitude: 0, Longitude: 0.003},
			{ID: 5, Name: "Intermediate stop", Latitude: 0, Longitude: 0.0015},
		},
		[]model.Edge{
			{From: 1, To: 2, Mode: model.ModeTrain, DurationMinutes: 1, DistanceMeters: 500, Service: "T8"},
			{From: 2, To: 3, Mode: model.ModeTrain, DurationMinutes: 4, DistanceMeters: 2000, Service: "T4"},
			{From: 3, To: 4, Mode: model.ModeTrain, DurationMinutes: 1, DistanceMeters: 500, Service: "T8"},
			{From: 2, To: 5, Mode: model.ModeTrain, DurationMinutes: 2, DistanceMeters: 1000, Service: "T8"},
			{From: 5, To: 3, Mode: model.ModeTrain, DurationMinutes: 2, DistanceMeters: 1000, Service: "T8"},
		},
	)

	route := routeFromEdges(network, repairServiceReentries(network, []model.Edge{
		{From: 1, To: 2, Mode: model.ModeTrain, DurationMinutes: 1, DistanceMeters: 500, Service: "T8"},
		{From: 2, To: 3, Mode: model.ModeTrain, DurationMinutes: 4, DistanceMeters: 2000, Service: "T4"},
		{From: 3, To: 4, Mode: model.ModeTrain, DurationMinutes: 1, DistanceMeters: 500, Service: "T8"},
	}))
	if route.Transfers != 0 || len(route.Steps) != 4 {
		t.Fatalf("route = %+v, want a continuous four-edge T8 route", route)
	}
	for _, step := range route.Steps {
		if step.Service != "T8" {
			t.Fatalf("step = %+v, want T8 service", step)
		}
	}
}

func TestRouteHasShortTransitRun(t *testing.T) {
	short := model.Route{Steps: []model.RouteStep{{Mode: model.ModeBus, DurationMinutes: 1, Service: "B1"}}}
	if !routeHasShortTransitRun(short) {
		t.Fatal("one-minute transit run was not rejected")
	}

	continuous := model.Route{Steps: []model.RouteStep{
		{Mode: model.ModeBus, DurationMinutes: 1, Service: "B1"},
		{Mode: model.ModeBus, DurationMinutes: 1, Service: "B1"},
	}}
	if routeHasShortTransitRun(continuous) {
		t.Fatal("continuous two-minute transit run was rejected")
	}
}

func TestWalkingWaypointRouteMeetsMinimum(t *testing.T) {
	network := mustGraph(t,
		[]model.Node{
			{ID: 1, Name: "Start", Latitude: 0, Longitude: 0},
			{ID: 2, Name: "Walking waypoint", Latitude: 0, Longitude: 0.001},
			{ID: 3, Name: "Goal", Latitude: 0, Longitude: 0.002},
		},
		[]model.Edge{
			{From: 1, To: 3, Mode: model.ModeBus, DurationMinutes: 1, DistanceMeters: 1000, Service: "B1"},
			{From: 1, To: 2, Mode: model.ModeWalk, DurationMinutes: 6, DistanceMeters: 500},
			{From: 2, To: 3, Mode: model.ModeBus, DurationMinutes: 2, DistanceMeters: 1000, Service: "B1"},
		},
	)

	minimum := 5.0
	route, err := findRouteWithWalkingWaypoint(network, model.RouteRequest{From: 1, To: 3, MinWalkingMinutes: &minimum})
	if err != nil {
		t.Fatalf("findRouteWithWalkingWaypoint returned error: %v", err)
	}
	if route.WalkingMinutes < minimum || route.TotalMinutes != 8 {
		t.Fatalf("route = %+v, want at least five walking minutes and eight total minutes", route)
	}
}

func TestFindRouteCountsServiceTransfers(t *testing.T) {
	network := mustGraph(t,
		[]model.Node{
			{ID: 1, Name: "Start", Latitude: 0, Longitude: 0},
			{ID: 2, Name: "Bus stop", Latitude: 0, Longitude: 0.001},
			{ID: 3, Name: "Rail station", Latitude: 0, Longitude: 0.002},
			{ID: 4, Name: "Goal", Latitude: 0, Longitude: 0.003},
		},
		[]model.Edge{
			{From: 1, To: 2, Mode: model.ModeWalk, DurationMinutes: 1, DistanceMeters: 80},
			{From: 2, To: 3, Mode: model.ModeBus, DurationMinutes: 5, DistanceMeters: 1000, Service: "B1"},
			{From: 3, To: 4, Mode: model.ModeTrain, DurationMinutes: 6, DistanceMeters: 1200, Service: "T1"},
		},
	)

	route, err := FindRoute(network, model.RouteRequest{From: 1, To: 4, MinWalkingMinutes: minutes(1)})
	if err != nil {
		t.Fatalf("FindRoute returned error: %v", err)
	}
	if route.Transfers != 1 {
		t.Fatalf("transfers = %d, want 1", route.Transfers)
	}
	if route.WalkingMeters != 80 {
		t.Fatalf("walking meters = %v, want 80", route.WalkingMeters)
	}
}

func TestFindRouteStartEqualsDestination(t *testing.T) {
	network := mustGraph(t,
		[]model.Node{{ID: 1, Name: "Start", Latitude: 0, Longitude: 0}},
		nil,
	)

	route, err := FindRoute(network, model.RouteRequest{From: 1, To: 1, MinWalkingMinutes: minutes(0)})
	if err != nil {
		t.Fatalf("FindRoute returned error: %v", err)
	}
	if route.TotalMinutes != 0 || len(route.Steps) != 0 {
		t.Fatalf("route = %+v", route)
	}
}

func TestDistanceBetweenKnownCoordinates(t *testing.T) {
	distance := DistanceBetween(
		model.Node{Latitude: 0, Longitude: 0},
		model.Node{Latitude: 0, Longitude: 1},
	)
	if distance < 111_000 || distance > 112_000 {
		t.Fatalf("distance = %f, want approximately 111,195 metres", distance)
	}
}

func mustGraph(t *testing.T, nodes []model.Node, edges []model.Edge) *graph.Graph {
	t.Helper()
	network, err := graph.New(nodes, edges)
	if err != nil {
		t.Fatalf("graph.New returned error: %v", err)
	}
	return network
}

func minutes(value float64) *float64 {
	return &value
}
