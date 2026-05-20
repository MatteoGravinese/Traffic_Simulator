package main

import (
	"fmt"
	"log"

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
}
