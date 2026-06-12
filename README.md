# Traffic Simulator

Real-time traffic simulation on OpenStreetMap road networks, built in Go with a Leaflet.js frontend. Load a region, spawn vehicles, and watch congestion build up on the map as agents route through the network.

## Features

- **OSM network loading** — draw a bounding box (max 0.3° per side) or upload a `.osm` file
- **Fast routing** — Contraction Hierarchies (CH) for initial paths; Customizable Contraction Hierarchies (CCH) when traffic-aware mode is on
- **Congestion model** — density from live vehicle counts per road segment, smoothed with an exponential moving average (EMA) for visualization and routing
- **Traffic-aware rerouting** — optional CCH edge-weight customization from EMA congestion, with round-robin updates (~30 vehicles per tick) to limit CPU cost
- **Congestion-adjusted movement** — vehicles slow on crowded segments using instantaneous density (`speed / (1 + 3×density)`)
- **Live map** — road segments colored by congestion level; distribution chart and traffic-aware toggle in the HUD

Pathfinding in `internal/graph/` uses **Dijkstra** as a benchmark baseline, plus **CH** and **CCH** for production routing. The live simulation uses **CH** and **CCH** only.

## Requirements

- Go 1.26+
- A modern browser
- Network access for loading map regions and for the first benchmark run (Overpass + Nominatim)

No database or Redis required.

## Run

### Web app

```bash
go run cmd/main.go
```

Open [http://localhost:8080](http://localhost:8080), then:

1. Draw a selection area on the map (or upload an `.osm` file) and click **Confirm & Load Network**.
2. After preprocessing finishes, use **Spawn Vehicles** in the sidebar HUD.
3. Toggle **Traffic-Aware Routing** to enable CCH-based rerouting around congested edges.

### Benchmark

Compare Dijkstra, CH, and CCH on the Pittsburgh metro network. Downloads `testdata/Pittsburgh.osm` on first run (cached locally, gitignored) and refreshes the [benchmark results](#pathfinding-benchmark-pittsburghosm) below.

```bash
go run ./cmd/benchmark
```

## Architecture

```
Browser (Leaflet)  ←HTTP→  Go API server  ←in-memory→  SimState
                              ↑
                         tick loop (100ms)
                         · update congestion (raw + EMA)
                         · optional CCH reroute (EMA weights)
                         · move vehicles (raw congestion speed)
```

| Data | Used for |
|------|----------|
| `Congestion` (raw) | Vehicle movement speed |
| `CongestionEMA` | Map colors, `/congestion` API, traffic-aware routing |

## API endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /roads` | Road segments in bounding box (zoom-based detail) |
| `GET /congestion` | EMA congestion values in bounding box |
| `GET /api/load-area` | SSE: download/parse OSM, build CH/CCH |
| `GET /api/spawn-vehicles` | SSE: spawn vehicle fleet |
| `POST /config/traffic-aware` | Enable/disable traffic-aware routing |
| `POST /api/upload-osm` | Upload `.osm` file for parsing |

## Project layout

```
cmd/main.go           Server entry point and tick loop
cmd/benchmark/        Pathfinding benchmark runner
testdata/             Local OSM cache (gitignored; Pittsburgh.osm fetched on first benchmark run)
internal/api/         HTTP server and OSM load pipeline
internal/graph/       Graph, parsers, pathfinding (Dijkstra, CH, CCH)
internal/osmpath/     Shared OSM directory paths
internal/simulation/  Vehicles, congestion, tick logic
web/                  Single-page Leaflet UI
```

<!-- BENCHMARK:START -->
## Pathfinding benchmark (Pittsburgh.osm)

Dataset: `testdata/Pittsburgh.osm` (Pittsburgh drivable highways via Nominatim + Overpass area query; downloaded on first run, gitignored). Re-run with `go run ./cmd/benchmark`.

|     Metric     |   Value    |
| :------------: | :--------: |
| Last run (UTC) | 2026-06-12 |
|     Nodes      |   88,514   |
| Directed edges |  175,011   |
| CH preprocess  |   5.36s    |
| CCH preprocess |   2.197s   |

### Single-query latency (static weights)

Averaged over 100 random origin–destination pairs (seed=42). Useful for one-off routing on a fixed graph.

|   Algorithm    | Avg query time | Speedup vs Dijkstra |
| :------------: | :------------: | :-----------------: |
|    Dijkstra    |    49.77ms     |        1.0×        |
|       CH       |     1.33ms     |       37.4×        |
| CCH query only |    2.713ms     |       18.3×        |

CCH query-only timing uses one free-flow customization before the loop; isolated CCH queries are not the intended production path.

### Traffic-aware tick (30 reroutes per tick)

Models one simulation tick with traffic-aware rerouting: **30 Dijkstra queries** vs **one Customize (random 5% edge congestion) + 30 CCH queries**, averaged over 20 ticks. Matches `maxVehiclesPerTick` in the simulator.

|       Approach        |             Avg tick time             | Speedup vs Dijkstra tick |
| :-------------------: | :-----------------------------------: | :----------------------: |
|    Dijkstra × 30     |                1.274s                 |          1.0×           |
|       CH × 30        |          **DOES NOT APPLY**           |    **DOES NOT APPLY**    |
| Customize + CCH × 30 | 121ms (customize 40ms + queries 81ms) |          10.5×          |

CH does not apply to traffic-aware ticks: it is preprocessed for static edge weights and cannot be re-customized each tick when congestion changes.

**Takeaway:** CH is fastest when weights are static. CCH is for traffic-aware ticks — one Customize per tick plus batch reroutes beats running Dijkstra on every vehicle while still reacting to congestion.
<!-- BENCHMARK:END -->

## Author

Matteo Gravinese
