# WalkRoute

WalkRoute is a Go navigation demo that uses A* search to find routes with a required minimum amount of walking. It supports walking, buses, and trains in the demo network.

## Requirements

- Go 1.25 or newer
- Git LFS

## Get the Network Data

The runtime graph is stored in `data/network.json` with Git LFS because it is about 233 MB.

After cloning the repository, run:

```bash
git lfs install
git lfs pull origin main
git lfs checkout data/network.json
```

On Arch Linux, install Git LFS first if `git lfs` is not available:

```bash
sudo pacman -S git-lfs
```

The downloaded file should be about 233 MB:

```bash
stat -c '%s bytes' data/network.json
```

If the server reports `invalid character 'v' looking for beginning of value`, `network.json` is still an LFS pointer. Run the LFS commands above and try again.

## Run

From the repository root:

```bash
go run ./cmd/server
```

Open [http://localhost:8080](http://localhost:8080).

Useful server flags:

```bash
go run ./cmd/server -data data/network.json -addr :9090
```

## Import Network Data

The importer can build a new network from OpenStreetMap data:

```bash
go run ./cmd/importer \
  -osm-file data/raw/osm.json \
  -bbox=-33.895,151.190,-33.865,151.220 \
  -output=data/network.json
```

To include a local GTFS feed, add `-gtfs data/raw/feed.zip`. The generated `data/network.json` can be used with the server. Large raw GTFS files are intentionally not committed.

## API

Get available locations:

```text
GET /api/locations
```

Find a route:

```bash
curl -X POST http://localhost:8080/api/route \
  -H 'Content-Type: application/json' \
  -d '{"from":1,"to":5,"minWalkingMinutes":30}'
```

`minWalkingMinutes` is a hard minimum. The router minimizes total travel time while meeting it.

## How It Works

A* searches over the current location, walking progress, and the current public transport service. The route response includes total time, walking time, walking distance, transfers, and the number of explored states.

## Verify

```bash
go test ./...
go vet ./...
go build ./...
```
