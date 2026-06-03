package api

import (
	"context"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/MatteoGravinese/Traffic_Simulator/internal/graph"
	"github.com/MatteoGravinese/Traffic_Simulator/internal/simulation"
)

type progressReader struct {
	r           io.Reader
	total       int64
	read        int64
	send        func(float64)
	lastPercent int
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	if n > 0 && pr.total > 0 {
		pr.read += int64(n)
		percent := int(float64(pr.read) * 100.0 / float64(pr.total))
		if percent != pr.lastPercent && percent%5 == 0 {
			pr.lastPercent = percent
			pr.send(float64(percent) / 100.0)
		}
	}
	return n, err
}

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
	http.HandleFunc("/api/spawn-vehicles", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}
		sendProgress := func(status string, message string, progress float64) {
			event := map[string]interface{}{
				"status":  status,
				"message": message,
			}
			if progress >= 0 {
				event["progress"] = progress
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			flusher.Flush()
		}
		q := r.URL.Query()
		vehicleCount, err := strconv.Atoi(q.Get("vehicleCount"))
		if err != nil || vehicleCount <= 0 {
			vehicleCount = 50
		}
		if state.G == nil {
			sendProgress("error", "No loaded network available. Load an area first.", -1)
			return
		}
		sendProgress("initializing", "Spawning vehicles and launching simulation...", 0.0)
		state.SpawnVehicles(vehicleCount, func(p float64) {
			sendProgress("initializing", "Spawning vehicles and launching simulation...", p)
		})
		sendProgress("done", fmt.Sprintf("Spawned %d vehicles.", vehicleCount), 1.0)
	})
	http.HandleFunc("/api/upload-osm", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get file: %v", err), http.StatusBadRequest)
			return
		}
		defer file.Close()

		if err := os.MkdirAll("data", 0755); err != nil {
			http.Error(w, fmt.Sprintf("Failed to create data directory: %v", err), http.StatusInternalServerError)
			return
		}

		tempFile, err := os.CreateTemp("data", "uploaded-*.osm")
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create temp file: %v", err), http.StatusInternalServerError)
			return
		}
		tempFilePath := tempFile.Name()
		defer tempFile.Close()

		_, err = io.Copy(tempFile, file)
		if err != nil {
			os.Remove(tempFilePath)
			http.Error(w, fmt.Sprintf("Failed to save uploaded file: %v", err), http.StatusInternalServerError)
			return
		}

		baseName := filepath.Base(tempFilePath)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"temp_file": baseName,
		})
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
		sendProgress := func(status string, message string, progress float64) {
			event := map[string]interface{}{
				"status":  status,
				"message": message,
			}
			if progress >= 0 {
				event["progress"] = progress
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			flusher.Flush()
		}
		q := r.URL.Query()
		fileParam := q.Get("file")

		var tempFilePath string

		if fileParam != "" {
			// Using uploaded file
			tempFilePath = filepath.Join("data", filepath.Base(fileParam))
			if _, err := os.Stat(tempFilePath); os.IsNotExist(err) {
				sendProgress("error", "Uploaded file not found. Please upload again.", -1)
				return
			}
			sendProgress("downloading", "Using uploaded map data...", 1.0)
		} else {
			// Bounding box area load from Overpass API
			minLat, err1 := strconv.ParseFloat(q.Get("minLat"), 64)
			minLon, err2 := strconv.ParseFloat(q.Get("minLon"), 64)
			maxLat, err3 := strconv.ParseFloat(q.Get("maxLat"), 64)
			maxLon, err4 := strconv.ParseFloat(q.Get("maxLon"), 64)
			if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
				sendProgress("error", "Invalid coordinates provided.", -1)
				return
			}
			// Enforce safety limits (increased to 0.3 x 0.3).
			latSide := maxLat - minLat
			if latSide < 0 {
				latSide = -latSide
			}
			lonSide := maxLon - minLon
			if lonSide < 0 {
				lonSide = -lonSide
			}
			if latSide <= 0 || lonSide <= 0 {
				sendProgress("error", "Invalid bounding box dimensions.", -1)
				return
			}
			if latSide > 0.3 || lonSide > 0.3 {
				sendProgress("error", "Selected area is too large. Maximum size is 0.3 x 0.3 degrees.", -1)
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

			// Download the OSM file using multi-mirror failover
			if err := os.MkdirAll("data", 0755); err != nil {
				sendProgress("error", fmt.Sprintf("Failed to create data directory: %v", err), -1)
				return
			}
			tempFile, err := os.CreateTemp("data", "active-*.osm")
			if err != nil {
				sendProgress("error", fmt.Sprintf("Failed to create temp file: %v", err), -1)
				return
			}
			tempFilePath = tempFile.Name()
			tempFile.Close()

			overpassQuery := fmt.Sprintf(`[out:xml][timeout:60];(way["highway"](%f,%f,%f,%f););(._;>;);out body;`, south, west, north, east)
			// Parallel download from mirrors (first successful wins)
			mirrors := []string{
				"https://overpass-api.de/api/interpreter",
			}
			// Context to cancel remaining requests once one succeeds
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			type result struct {
				resp *http.Response
				err  error
				host string
			}
			resCh := make(chan result, len(mirrors))

			for _, mirrorURL := range mirrors {
				go func(mirror string) {
					mirrorHost := mirror
					u, err := url.Parse(mirror)
					if err == nil {
						mirrorHost = u.Host
					}
					sendProgress("downloading", fmt.Sprintf("Downloading map data from OSM (%s)...", mirrorHost), 0.0)

					client := &http.Client{Timeout: 60 * time.Second}
					data := url.Values{}
					data.Set("data", overpassQuery)
					req, err := http.NewRequestWithContext(ctx, "POST", mirror, strings.NewReader(data.Encode()))
					if err != nil {
						resCh <- result{nil, err, mirrorHost}
						return
					}
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					req.Header.Set("User-Agent", "TrafficSimulator/1.0 (matteogravinese@github.com)")

					resp, err := client.Do(req)
					if err != nil {
						resCh <- result{nil, err, mirrorHost}
						return
					}
					if resp.StatusCode != http.StatusOK {
						resp.Body.Close()
						resCh <- result{nil, fmt.Errorf("Overpass API error (HTTP %s) from %s", resp.Status, mirrorHost), mirrorHost}
						return
					}
					resCh <- result{resp, nil, mirrorHost}
				}(mirrorURL)
			}

			var resp *http.Response
			var downloadErr error
			for i := 0; i < len(mirrors); i++ {
				r := <-resCh
				if r.err == nil {
					resp = r.resp
					
					break
				} else {
					downloadErr = r.err
				}
			}
			if resp == nil {
				downloadErr = fmt.Errorf("failed to download map data: %v", downloadErr)
			}

			if downloadErr != nil {
				os.Remove(tempFilePath)
				sendProgress("error", fmt.Sprintf("Failed to download map data: %v. Please try again or upload an OSM file.", downloadErr), -1)
				return
			}
			defer resp.Body.Close()

			out, err := os.Create(tempFilePath)
			if err != nil {
				sendProgress("error", fmt.Sprintf("Failed to create output file: %v", err), -1)
				return
			}
			reader := &progressReader{
				r:     resp.Body,
				total: resp.ContentLength,
				send: func(p float64) {
					sendProgress("downloading", "Downloading map data from OpenStreetMap...", p)
				},
			}
			_, err = io.Copy(out, reader)
			out.Close()
			if err != nil {
				os.Remove(tempFilePath)
				sendProgress("error", fmt.Sprintf("Failed to save downloaded data: %v", err), -1)
				return
			}
			// Cancel other mirror requests now that we have the data
			cancel()
			// Report completion progress
			sendProgress("downloading", "Downloading map data from OpenStreetMap...", 1.0)
		}

		// Delete the file when done parsing
		defer os.Remove(tempFilePath)

		// Parse the OSM file.
		sendProgress("parsing", "Parsing XML data and building road network graph...", 0.0)
		g, err := graph.ParseOSM(tempFilePath, func(p float64) {
			sendProgress("parsing", "Parsing XML data and building road network graph...", p)
		})
		if err != nil {
			sendProgress("error", fmt.Sprintf("Failed to parse OSM file: %v", err), -1)
			return
		}
		if len(g.Nodes) == 0 {
			sendProgress("error", "No routable road segments found in this network file.", -1)
			return
		}

		// Calculate actual bounding box of parsed graph
		var minLat, minLon, maxLat, maxLon float64
		firstNode := true
		for _, node := range g.Nodes {
			if firstNode {
				minLat, maxLat = node.Lat, node.Lat
				minLon, maxLon = node.Lon, node.Lon
				firstNode = false
			} else {
				if node.Lat < minLat {
					minLat = node.Lat
				}
				if node.Lat > maxLat {
					maxLat = node.Lat
				}
				if node.Lon < minLon {
					minLon = node.Lon
				}
				if node.Lon > maxLon {
					maxLon = node.Lon
				}
			}
		}

		// Preprocessing CH.
		edgeCount := g.EdgeCount()
		sendProgress("preprocessing_ch", fmt.Sprintf("Preprocessing Contraction Hierarchies (%d nodes, %d edges)...", len(g.Nodes), edgeCount), 0.0)
		ch := graph.Preprocess(g, func(p float64) {
			sendProgress("preprocessing_ch", fmt.Sprintf("Preprocessing Contraction Hierarchies (%d nodes, %d edges)...", len(g.Nodes), edgeCount), p)
		})

		// Preprocess CCH.
		sendProgress("preprocessing_cch", "Preprocessing Customizable Contraction Hierarchies...", 0.0)
		cch := graph.PreprocessCCH(g, func(p float64) {
			sendProgress("preprocessing_cch", "Preprocessing Customizable Contraction Hierarchies...", p)
		})
		cch.Customize(map[int64]map[int64]float64{})

		// Initialize the simulation state without spawning vehicles.
		sendProgress("initializing", "Loading road network into simulation state...", 0.0)
		state.UpdateGraph(g, ch, cch, 0.0, func(p float64) {
			sendProgress("initializing", "Loading road network into simulation state...", p)
		})

		// For done progress event, return the calculated coordinates so map can center:
		event := map[string]interface{}{
			"status":  "done",
			"message": "Network loaded successfully!",
			"progress": 1.0,
			"minLat":  minLat,
			"minLon":  minLon,
			"maxLat":  maxLat,
			"maxLon":  maxLon,
		}
		data, _ := json.Marshal(event)
		fmt.Fprintf(w, "data: %s\n\n", string(data))
		flusher.Flush()
	})
	http.Handle("/", http.FileServer(http.Dir("web")))
	http.ListenAndServe(":8080", nil)
}
