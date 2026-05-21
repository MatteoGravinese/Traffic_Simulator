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
	j := 0
	for j < 10 {
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
		fmt.Printf("Starting A*:\n")
		fmt.Printf("Finding path from %d to %d...\n", startID, endID)
		start_A := time.Now()
		path_A, distance_A, err := graph.AStar(g, startID, endID)
		elapsed_A := time.Since(start_A)
		if err != nil {
			log.Fatal("A* failed:", err)
		}
		fmt.Printf("Path found. %d nodes, %.2f miles in %v\n", len(path_A), distance_A, elapsed_A)
		fmt.Printf("Starting New Bidirectional A*:\n")
		fmt.Printf("Finding path from %d to %d...\n", startID, endID)
		start_New_Bidirectional_A := time.Now()
		path_New_Bidirectional_A, distance_New_Bidirectional_A, err := graph.NewBidirectionalAStar(g, startID, endID)
		elapsed_New_Bidirectional_A := time.Since(start_New_Bidirectional_A)
		if err != nil {
			log.Fatal("New Bidirectional A* failed:", err)
		}
		fmt.Printf("Path found. %d nodes, %.2f miles in %v\n", len(path_New_Bidirectional_A), distance_New_Bidirectional_A, elapsed_New_Bidirectional_A)
		j = j + 1
	}
}
