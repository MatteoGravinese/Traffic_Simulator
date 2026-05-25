package api

import (
	"compress/gzip"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/MatteoGravinese/Traffic_Simulator/internal/graph"
	"github.com/MatteoGravinese/Traffic_Simulator/internal/simulation"
)

type EdgeCongestion struct {
	FromLat    float64 `json:"from_lat"`
	FromLon    float64 `json:"from_lon"`
	ToLat      float64 `json:"to_lat"`
	ToLon      float64 `json:"to_lon"`
	Congestion float64 `json:"congestion"`
}

type RoadSegment [4]float64

type gzipResponseWriter struct {
	http.ResponseWriter
	Writer *gzip.Writer
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	return g.Writer.Write(b)
}

func withGzip(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			h(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		h(&gzipResponseWriter{ResponseWriter: w, Writer: gz}, r)
	}
}

var highwayPriority = map[string]int{
	"motorway":       0,
	"motorway_link":  0,
	"trunk":          1,
	"trunk_link":     1,
	"primary":        2,
	"primary_link":   2,
	"secondary":      3,
	"secondary_link": 3,
	"tertiary":       4,
	"tertiary_link":  4,
	"residential":    5,
	"living_street":  5,
	"unclassified":   5,
	"service":        6,
}

func minPriorityForZoom(zoom int) int {
	switch {
	case zoom >= 17:
		return 6
	case zoom >= 15:
		return 4
	case zoom >= 13:
		return 2
	default:
		return 1
	}
}

func StartServer(g *graph.Graph, state *simulation.SimState) {
	type edgeInfo struct {
		priority int
	}
	edgeInfoMap := make(map[int64]map[int64]edgeInfo)
	for fromID, edges := range g.Edges {
		edgeInfoMap[fromID] = make(map[int64]edgeInfo)
		for _, edge := range edges {
			priority := 6
			if p, ok := highwayPriority[edge.Highway]; ok {
				priority = p
			}
			edgeInfoMap[fromID][edge.To] = edgeInfo{priority: priority}
		}
	}
	http.HandleFunc("/roads", withGzip(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		minLat, _ := strconv.ParseFloat(q.Get("minLat"), 64)
		minLon, _ := strconv.ParseFloat(q.Get("minLon"), 64)
		maxLat, _ := strconv.ParseFloat(q.Get("maxLat"), 64)
		maxLon, _ := strconv.ParseFloat(q.Get("maxLon"), 64)
		zoom, _ := strconv.Atoi(q.Get("zoom"))
		maxPriority := minPriorityForZoom(zoom)
		segments := make([]RoadSegment, 0)
		for fromID, edges := range g.Edges {
			fromNode, ok := g.Nodes[fromID]
			if !ok {
				continue
			}
			for _, edge := range edges {
				info := edgeInfoMap[fromID][edge.To]
				if info.priority > maxPriority {
					continue
				}
				toNode, ok := g.Nodes[edge.To]
				if !ok {
					continue
				}
				fromInBounds := fromNode.Lat >= minLat && fromNode.Lat <= maxLat &&
					fromNode.Lon >= minLon && fromNode.Lon <= maxLon
				toInBounds := toNode.Lat >= minLat && toNode.Lat <= maxLat &&
					toNode.Lon >= minLon && toNode.Lon <= maxLon
				if !fromInBounds && !toInBounds {
					continue
				}
				segments = append(segments, RoadSegment{
					fromNode.Lat, fromNode.Lon,
					toNode.Lat, toNode.Lon,
				})
			}
		}
		json.NewEncoder(w).Encode(segments)
	}))
	http.HandleFunc("/congestion", withGzip(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		minLat, _ := strconv.ParseFloat(q.Get("minLat"), 64)
		minLon, _ := strconv.ParseFloat(q.Get("minLon"), 64)
		maxLat, _ := strconv.ParseFloat(q.Get("maxLat"), 64)
		maxLon, _ := strconv.ParseFloat(q.Get("maxLon"), 64)
		state.RLock()
		edges := make([]EdgeCongestion, 0)
		// Changed data source loop to use CongestionEMA instead of Congestion.
		for fromID, toMap := range state.CongestionEMA {
			fromNode, ok := g.Nodes[fromID]

			if !ok {
				continue
			}
			for toID, congestion := range toMap {
				toNode, ok := g.Nodes[toID]
				if !ok {
					continue
				}
				// Shifted bounding checks down to analyze both nodes.
				fromInBounds := fromNode.Lat >= minLat && fromNode.Lat <= maxLat &&
					fromNode.Lon >= minLon && fromNode.Lon <= maxLon
				toInBounds := toNode.Lat >= minLat && toNode.Lat <= maxLat &&
					toNode.Lon >= minLon && toNode.Lon <= maxLon
				if !fromInBounds && !toInBounds {
					continue
				}
				edges = append(edges, EdgeCongestion{
					FromLat:    fromNode.Lat,
					FromLon:    fromNode.Lon,
					ToLat:      toNode.Lat,
					ToLon:      toNode.Lon,
					Congestion: congestion,
				})
			}
		}
		state.RUnlock()
		json.NewEncoder(w).Encode(edges)
	}))
	http.HandleFunc("/config/traffic-aware", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		enabledStr := r.URL.Query().Get("enabled")
		enabled, err := strconv.ParseBool(enabledStr)
		if err != nil {
			http.Error(w, "Invalid 'enabled' value", http.StatusBadRequest)
			return
		}
		state.SetTrafficAware(enabled)
		w.WriteHeader(http.StatusOK)
	})
	http.Handle("/", http.FileServer(http.Dir("web")))
	http.ListenAndServe(":8080", nil)
}
