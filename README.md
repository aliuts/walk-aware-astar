# WalkRoute

WalkRoute is a small multimodal navigation demo written in Go. It uses A* search to choose a route according to a user-selected objective while requiring a user-selected minimum amount of walking.

The important detail is the search state. A location alone is not enough because two paths can reach the same location with different walking progress and transit context. WalkRoute therefore searches over:

```text
(current location, walking progress, last public transport service)
```

The graph is intentionally small and uses representative locations around UTS, Ultimo, Haymarket, Central, and Town Hall. It is not connected to live transport data.

The repository also includes an importer for creating a replacement network from raw OpenStreetMap and GTFS data. The checked-in demo network remains available so the application works without a download.

## Run

Requirements:

- Go 1.25 or newer

Start the server from the repository root:

```bash
go run ./cmd/server
```

Then open [http://localhost:8080](http://localhost:8080).

The network file and listen address can be changed with flags:

```bash
go run ./cmd/server -data data/network.json -addr :9090
```

## Import Raw Network Data

The importer builds the same `network.json` format consumed by the server. It does not call a routing service or import completed routes.

### Offline OSM import

Save an Overpass JSON response in `data/raw/osm.json`, then run:

```bash
go run ./cmd/importer \
  -osm-file data/raw/osm.json \
  -bbox -33.895,151.190,-33.865,151.220 \
  -output data/network.json
```

When `-osm-file` is omitted, the importer downloads pedestrian ways from the Overpass API. The default query requests all `highway=*` ways within the bounding box, then the importer applies its own pedestrian-access policy.

For a larger area, split the Overpass request into overlapping tiles. Tile elements are deduplicated before the walking graph is built:

```bash
go run ./cmd/importer \
  -bbox=-33.99,150.88,-33.86,151.23 \
  -osm-tile-size=0.05 \
  -gtfs=data/raw/gtfs.zip \
  -output=data/network.json \
  -timeout=30m
```

If an individual tile still returns a gateway timeout, reduce `-osm-tile-size` to `0.03` or use another Overpass instance with `-overpass-url`.

The initial policy accepts `footway`, `pedestrian`, `path`, `track`, `steps`, `living_street`, `residential`, `service`, `unclassified`, `tertiary`, `secondary`, `primary`, and `cycleway`. Private or explicitly `foot=no` ways are rejected. Walking edges use a default speed of `1.4 m/s` and preserve OSM node IDs. Named OSM points and named ways become user-facing locations; anonymous geometry vertices remain available to the router but are hidden from the location picker when enough named locations exist.

### Optional GTFS import

Add a local GTFS ZIP containing `stops.txt`, `routes.txt`, `trips.txt`, and `stop_times.txt`:

```bash
go run ./cmd/importer \
  -osm-file data/raw/osm.json \
  -gtfs data/raw/feed.zip \
  -bbox -33.895,151.190,-33.865,151.220 \
  -output data/network.json
```

For Sydney buses and trains, TfNSW publishes a complete static GTFS bundle. The official API endpoint is:

```text
https://api.transport.nsw.gov.au/v1/publictransport/timetables/complete/gtfs
```

It requires a TfNSW API key using the `Authorization: apikey ...` header. The importer can download and cache it directly:

```bash
export TFNSW_API_KEY="your-key"
go run ./cmd/importer \
  -gtfs-url https://api.transport.nsw.gov.au/v1/publictransport/timetables/complete/gtfs \
  -gtfs-api-key "$TFNSW_API_KEY" \
  -bbox=-33.895,151.190,-33.865,151.220 \
  -output data/network.json
```

The ZIP is saved as `data/raw/gtfs.zip`. The complete bundle contains static schedules for multiple operators, including Sydney buses and trains. This application converts those schedules into static in-vehicle edges; it does not yet model departure waiting time.

The importer converts supported GTFS route types into static bus or train edges, chooses the fastest observed trip between consecutive stops, and connects each stop to its nearest OSM node within 500 metres. GTFS timetable waiting is deliberately not included yet, so transit edge durations represent in-vehicle travel only.

Useful importer flags include `-overpass-url`, `-osm-tile-size`, `-walking-speed`, `-max-transfer-distance`, `-user-agent`, and `-timeout`.

## API

### `GET /api/locations`

Returns the available graph locations.

### `POST /api/route`

Request:

```json
{
  "from": 1,
  "to": 5,
  "minWalkingMinutes": 30
}
```

`minWalkingMinutes` is a hard lower bound; when omitted, it defaults to zero. Routes always minimize total travel time.

Successful response:

```json
{
  "steps": [],
  "totalMinutes": 42,
  "walkingMinutes": 30,
  "walkingMeters": 2400,
  "transfers": 0,
  "nodesExplored": 8
}
```

If no route can include the requested minimum walking time, the endpoint returns HTTP `422`:

```json
{
  "error": "no route found while meeting the minimum walking requirement"
}
```

## Algorithm

The priority of a state is:

```text
f(state) = travel time so far + geographic heuristic
```

The heuristic uses Haversine distance divided by the maximum observed edge speed in the loaded graph. Walking usage is stored as integer microseconds rather than a raw floating-point value, and the walking-progress state is capped once it reaches the requested minimum. A destination state is accepted only after its walking time reaches that minimum. A small boarding penalty prevents a short vehicle hop from beating a comparable walk, and service-transfer penalties keep a route on the same service when practical. `SHL` edges are excluded from routing. Large imported graphs use walking waypoints to keep high minimum-walking searches bounded and reject repeated-node detours.

The route reconstruction phase calculates:

- total travel time
- walking time
- walking distance
- transfers between public transport services
- the number of A* states explored

Walking edges are automatically made bidirectional when the reverse edge is not already present. Public transport edges remain directed unless both directions are included in `data/network.json`.

## Project Layout

```text
cmd/server/          HTTP server entrypoint
internal/model/      Shared graph and route types
internal/geo/        Geographic distance and bounding-box utilities
internal/graph/      Graph construction and JSON loading
internal/routing/    A* search, heuristic, queue, reconstruction
internal/api/        HTTP handlers and validation
internal/osm/        Overpass client and pedestrian graph conversion
internal/gtfs/       GTFS ZIP parsing and stop/network merging
cmd/importer/        Raw OSM/GTFS preprocessing command
web/                 Embedded browser application
data/raw/            Optional raw OSM and GTFS inputs
data/network.json    Runtime graph output
```

## Verification

```bash
go test ./...
go vet ./...
go build ./...
```

Timetable-aware routing is intentionally left as a future extension because waiting time makes edge costs depend on arrival time.
# walk-aware-astar
