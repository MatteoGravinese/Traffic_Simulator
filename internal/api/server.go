package api

import (
	"encoding/json"
	"net/http"

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

func StartServer(g *graph.Graph, state *simulation.SimState) {
	http.HandleFunc("/congestion", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		edges := make([]EdgeCongestion, 0)
		for fromID, toMap := range state.Congestion {
			fromNode, ok := g.Nodes[fromID]
			if !ok {
				continue
			}
			for toID, congestion := range toMap {
				toNode, ok := g.Nodes[toID]
				if !ok {
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
		json.NewEncoder(w).Encode(edges)
	})
	http.Handle("/", http.FileServer(http.Dir("web")))
	http.ListenAndServe(":8080", nil)
}
