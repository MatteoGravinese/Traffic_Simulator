package main

import (
	"fmt"
	"log"
	"time"

	"github.com/MatteoGravinese/Traffic_Simulator/internal/api"
	"github.com/MatteoGravinese/Traffic_Simulator/internal/graph"
)

func main() {
	fmt.Println("Traffic Simulator starting...")
	db := api.ConnectPostgres()
	defer db.Close()
	rdb := api.ConnectRedis()
	_ = rdb
	fmt.Println("Loading road network...")
	g, err := graph.ParseOSM("data/pittsburgh.osm")
	if err != nil {
		log.Fatal("Failed to parse OSM file:", err)
	}
	fmt.Printf("Graph loaded. %d nodes, %d edges\n", len(g.Nodes), len(g.Edges))
	//Grab any two node IDs from the graph to test with.
	var startID, endID int64
	i := 0
	for id := range g.Nodes {
		if i == 0 {
			startID = id
		}
		if i == 100 {
			endID = id
		}
		i++
		if i > 100 {
			break
		}
	}
	fmt.Printf("Finding path from %d to %d...\n", startID, endID)
	start := time.Now()
	path, distance, err := graph.Dijkstra(g, startID, endID)
	elapsed := time.Since(start)
	if err != nil {
		log.Fatal("Dijkstra failed:", err)
	}
	fmt.Printf("Path found. %d nodes, %.2f miles in %v\n", len(path), distance, elapsed)
	fmt.Printf("Starting A*\n")
	fmt.Printf("Finding path from %d to %d...\n", startID, endID)
	start2 := time.Now()
	path2, distance2, err2 := graph.AStar(g, startID, endID)
	elapsed2 := time.Since(start2)
	if err2 != nil {
		log.Fatal("A* failed:", err2)
	}
	fmt.Printf("Path found. %d nodes, %.2f miles in %v\n", len(path2), distance2, elapsed2)
}
