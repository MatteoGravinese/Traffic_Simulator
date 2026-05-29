package api

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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

func StartServer(state *simulation.SimState) {
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
		state.RLock()
		g := state.G
		priorities := state.EdgePriorities
		if g != nil {
			for fromID, edges := range g.Edges {
				fromNode, ok := g.Nodes[fromID]
				if !ok {
					continue
				}
				for _, edge := range edges {
					priority := 6
					if fromPrioMap, ok := priorities[fromID]; ok {
						if p, exists := fromPrioMap[edge.To]; exists {
							priority = p
						}
					}
					if priority > maxPriority {
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
		}
		state.RUnlock()
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
		g := state.G
		if g != nil {
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
	http.HandleFunc("/api/load-area", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}
		sendProgress := func(status string, message string) {
			event := map[string]string{
				"status":  status,
				"message": message,
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			flusher.Flush()
		}
		q := r.URL.Query()
		minLat, err1 := strconv.ParseFloat(q.Get("minLat"), 64)
		minLon, err2 := strconv.ParseFloat(q.Get("minLon"), 64)
		maxLat, err3 := strconv.ParseFloat(q.Get("maxLat"), 64)
		maxLon, err4 := strconv.ParseFloat(q.Get("maxLon"), 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			sendProgress("error", "Invalid coordinates provided.")
			return
		}
		// Enforce safety limits.
		latSide := maxLat - minLat
		if latSide < 0 {
			latSide = -latSide
		}
		lonSide := maxLon - minLon
		if lonSide < 0 {
			lonSide = -lonSide
		}
		if latSide <= 0 || lonSide <= 0 {
			sendProgress("error", "Invalid bounding box dimensions.")
			return
		}
		if latSide > 0.3 || lonSide > 0.3 {
			sendProgress("error", "Selected area is too large. Maximum size is 0.3 x 0.3 degrees.")
			return
		}
		// Normalize coordinate directions for Overpass API.
		south := minLat
		north := maxLat
		if south > north {
			south, north = north, south
		}
		west := minLon
		east := maxLon
		if west > east {
			west, east = east, west
		}
		//Download the OSM file.
		sendProgress("downloading", "Downloading map data from OpenStreetMap (Overpass API)...")
		if err := os.MkdirAll("data", 0755); err != nil {
			sendProgress("error", fmt.Sprintf("Failed to create data directory: %v", err))
			return
		}
		tempFile, err := os.CreateTemp("data", "active-*.osm")
		if err != nil {
			sendProgress("error", fmt.Sprintf("Failed to create temp file: %v", err))
			return
		}
		tempFilePath := tempFile.Name()
		tempFile.Close()
		defer os.Remove(tempFilePath)
		overpassUrl := "https://overpass-api.de/api/interpreter"
		overpassQuery := fmt.Sprintf(`[out:xml][timeout:60];(way["highway"](%f,%f,%f,%f););(._;>;);out body;`, south, west, north, east)
		client := &http.Client{}
		data := url.Values{}
		data.Set("data", overpassQuery)
		req, err := http.NewRequest("POST", overpassUrl, strings.NewReader(data.Encode()))
		if err != nil {
			sendProgress("error", fmt.Sprintf("Failed to create Overpass request: %v", err))
			return
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", "TrafficSimulator/1.0 (matteogravinese@github.com)")
		resp, err := client.Do(req)
		if err != nil {
			sendProgress("error", fmt.Sprintf("Failed to contact Overpass API: %v. Please try again.", err))
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			sendProgress("error", fmt.Sprintf("Overpass API error (HTTP %s). Try selecting a slightly smaller area.", resp.Status))
			return
		}
		out, err := os.Create(tempFilePath)
		if err != nil {
			sendProgress("error", fmt.Sprintf("Failed to create output file: %v", err))
			return
		}
		_, err = io.Copy(out, resp.Body)
		out.Close()
		if err != nil {
			sendProgress("error", fmt.Sprintf("Failed to save downloaded data: %v", err))
			return
		}
		// Parse the OSM file.
		sendProgress("parsing", "Parsing XML data and building road network graph...")
		g, err := graph.ParseOSM(tempFilePath)
		if err != nil {
			sendProgress("error", fmt.Sprintf("Failed to parse OSM file: %v", err))
			return
		}
		if len(g.Nodes) == 0 {
			sendProgress("error", "No routable road segments found in this bounding box. Draw a box with major streets or residential roads.")
			return
		}
		//Preprocessing CH.
		sendProgress("preprocessing_ch", fmt.Sprintf("Preprocessing Contraction Hierarchies (%d nodes, %d edges)...", len(g.Nodes), len(g.Edges)))
		ch := graph.Preprocess(g)
		//Preprocess CCH.
		sendProgress("preprocessing_cch", "Preprocessing Customizable Contraction Hierarchies...")
		cch := graph.PreprocessCCH(g)
		cch.Customize(map[int64]map[int64]float64{})
		//Initialize the Simulation State.
		sendProgress("initializing", "Spawning vehicles and launching simulation...")
		state.UpdateGraph(g, ch, cch)
		sendProgress("done", "Simulation started successfully!")
	})
	http.Handle("/", http.FileServer(http.Dir("web")))
	http.ListenAndServe(":8080", nil)
}
