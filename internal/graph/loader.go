package graph

import (
	"encoding/json"
	"fmt"
	"os"

	"walking-aware-nav/internal/model"
)

// LoadGraph reads and validates a network JSON file.
func LoadGraph(path string) (*Graph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read graph %q: %w", path, err)
	}

	var network model.NetworkFile
	if err := json.Unmarshal(data, &network); err != nil {
		return nil, fmt.Errorf("decode graph %q: %w", path, err)
	}
	if len(network.Nodes) == 0 {
		return nil, fmt.Errorf("graph %q contains no nodes", path)
	}

	graph, err := New(network.Nodes, network.Edges)
	if err != nil {
		return nil, fmt.Errorf("validate graph %q: %w", path, err)
	}
	return graph, nil
}
