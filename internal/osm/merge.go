package osm

import "sort"

type elementKey struct {
	typeName string
	id       int64
}

// MergeResponses combines tiled Overpass responses and removes elements that
// appear in more than one overlapping tile.
func MergeResponses(responses ...Response) Response {
	byKey := make(map[elementKey]Element)
	for _, response := range responses {
		for _, element := range response.Elements {
			key := elementKey{typeName: element.Type, id: element.ID}
			previous, exists := byKey[key]
			if !exists {
				byKey[key] = cloneElement(element)
				continue
			}

			if len(previous.Nodes) < len(element.Nodes) {
				previous.Nodes = append([]int64(nil), element.Nodes...)
			}
			if previous.Lat == 0 && previous.Lon == 0 && (element.Lat != 0 || element.Lon != 0) {
				previous.Lat = element.Lat
				previous.Lon = element.Lon
			}
			previous.Tags = mergeTags(previous.Tags, element.Tags)
			byKey[key] = previous
		}
	}

	keys := make([]elementKey, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].typeName != keys[j].typeName {
			return keys[i].typeName < keys[j].typeName
		}
		return keys[i].id < keys[j].id
	})

	merged := Response{Elements: make([]Element, 0, len(keys))}
	for _, key := range keys {
		merged.Elements = append(merged.Elements, byKey[key])
	}
	return merged
}

func cloneElement(element Element) Element {
	clone := element
	if element.Nodes != nil {
		clone.Nodes = append([]int64(nil), element.Nodes...)
	}
	clone.Tags = mergeTags(nil, element.Tags)
	return clone
}

func mergeTags(destination, source map[string]string) map[string]string {
	if destination == nil && source == nil {
		return nil
	}
	if destination == nil {
		destination = make(map[string]string, len(source))
	}
	for key, value := range source {
		if destination[key] == "" {
			destination[key] = value
		}
	}
	return destination
}
