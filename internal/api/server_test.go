package api

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"walking-aware-nav/internal/graph"
	"walking-aware-nav/internal/model"
)

func TestLocationsEndpoint(t *testing.T) {
	handler := testHandler(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/locations", nil)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Body.String(); got == "" || got[0] != '[' {
		t.Fatalf("body = %q, want JSON array", got)
	}
}

func TestRouteEndpoint(t *testing.T) {
	handler := testHandler(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/route", strings.NewReader(`{"from":1,"to":3,"minWalkingMinutes":1}`))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Body.String(); got == "" || got[0] != '{' {
		t.Fatalf("body = %q, want JSON object", got)
	}
}

func TestRouteEndpointReturnsUnprocessableForNoRoute(t *testing.T) {
	handler := testHandler(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/route", strings.NewReader(`{"from":1,"to":3,"minWalkingMinutes":2}`))
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "minimum walking requirement") {
		t.Fatalf("body = %s, want no-route message", recorder.Body.String())
	}
}

func TestNoRouteMessageExplainsWalkingOnlyNetworks(t *testing.T) {
	network, err := graph.New(
		[]model.Node{
			{ID: 1, Name: "A", Latitude: 0, Longitude: 0},
			{ID: 2, Name: "B", Latitude: 0, Longitude: 0.001},
		},
		[]model.Edge{{From: 1, To: 2, Mode: model.ModeWalk, DurationMinutes: 2, DistanceMeters: 100}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if message := noRouteMessage(network, model.RouteRequest{}); !strings.Contains(message, "walking data only") {
		t.Fatalf("message = %q, want walking-only diagnostic", message)
	}
}

func TestRouteEndpointRejectsUnknownFields(t *testing.T) {
	handler := testHandler(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/route", strings.NewReader(`{"from":1,"to":3,"minWalkingMinutes":1,"unexpected":true}`))
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	network, err := graph.New(
		[]model.Node{
			{ID: 1, Name: "Start", Latitude: 0, Longitude: 0},
			{ID: 2, Name: "Bus stop", Latitude: 0, Longitude: 0.001},
			{ID: 3, Name: "Goal", Latitude: 0, Longitude: 0.002},
		},
		[]model.Edge{
			{From: 1, To: 2, Mode: model.ModeWalk, DurationMinutes: 1, DistanceMeters: 80},
			{From: 2, To: 3, Mode: model.ModeBus, DurationMinutes: 5, DistanceMeters: 1000, Service: "B1"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	static := fstest.MapFS{"index.html": &fstest.MapFile{Mode: fs.FileMode(0o644), Data: []byte("ok")}}
	return NewHandler(network, static)
}
