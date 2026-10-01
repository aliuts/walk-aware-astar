package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"sort"
	"strings"

	"walking-aware-nav/internal/graph"
	"walking-aware-nav/internal/model"
	"walking-aware-nav/internal/routing"
)

const maxRequestBodyBytes = 1 << 20

// NewHandler creates the application HTTP handler.
func NewHandler(network *graph.Graph, staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/locations", locationsHandler(network))
	mux.HandleFunc("/api/route", routeHandler(network))
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	return mux
}

func locationsHandler(network *graph.Graph) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		locations := make([]model.Node, 0, len(network.Nodes))
		for _, node := range network.Nodes {
			locations = append(locations, node)
		}
		namedLocations := make([]model.Node, 0, len(locations))
		seenNames := make(map[string]struct{})
		for _, node := range locations {
			if !node.Named || strings.TrimSpace(node.Name) == "" {
				continue
			}
			name := strings.ToLower(strings.TrimSpace(node.Name))
			if _, seen := seenNames[name]; seen {
				continue
			}
			seenNames[name] = struct{}{}
			namedLocations = append(namedLocations, node)
		}
		if len(namedLocations) >= 2 {
			locations = namedLocations
		}
		sort.Slice(locations, func(i, j int) bool {
			if locations[i].Name != locations[j].Name {
				return locations[i].Name < locations[j].Name
			}
			return locations[i].ID < locations[j].ID
		})

		writeJSON(w, http.StatusOK, locations)
	}
}

func routeHandler(network *graph.Graph) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		var request model.RouteRequest
		if err := decoder.Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
			return
		}
		if err := ensureSingleJSONValue(decoder); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		route, err := routing.FindRoute(network, request)
		if err != nil {
			switch {
			case errors.Is(err, routing.ErrInvalidRequest):
				writeError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, routing.ErrNoRoute):
				writeError(w, http.StatusUnprocessableEntity, noRouteMessage(network, request))
			default:
				writeError(w, http.StatusInternalServerError, "route calculation failed")
			}
			return
		}

		writeJSON(w, http.StatusOK, route)
	}
}

func noRouteMessage(network *graph.Graph, request model.RouteRequest) string {
	message := routing.ErrNoRoute.Error()
	if request.MinWalkingMinutes != nil && *request.MinWalkingMinutes > 0 {
		message += " while meeting the minimum walking requirement"
	}
	for _, edges := range network.Edges {
		for _, edge := range edges {
			if edge.Mode == model.ModeBus || edge.Mode == model.ModeTrain {
				return message
			}
		}
	}
	message += "; this network contains walking data only, so import a GTFS feed"
	return message
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("invalid request body: multiple JSON values")
		}
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
