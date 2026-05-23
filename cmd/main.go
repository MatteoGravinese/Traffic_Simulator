package main

import (
	"fmt"
	"log"
	"math/rand"
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
	fmt.Println("Preprocessing graph for Contraction Hierarchies...")
	done := make(chan bool)
	go func() {
		chars := []string{"[          ]", "[=         ]", "[==        ]", "[===       ]", "[====      ]", "[=====     ]", "[======    ]", "[=======   ]", "[========  ]", "[========= ]", "[==========]"}
		i := 0
		for {
			select {
			case <-done:
				fmt.Print("\r                    \r")
				return
			default:
				fmt.Printf("\r%s", chars[i%len(chars)])
				i++
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()
	ch := graph.Preprocess(g)
	done <- true
	time.Sleep(100 * time.Millisecond)
	fmt.Println("Preprocessing complete.")
	fmt.Println("Preprocessing CCH...")
	doneCCH := make(chan bool)
	go func() {
		chars := []string{"[          ]", "[=         ]", "[==        ]", "[===       ]", "[====      ]", "[=====     ]", "[======    ]", "[=======   ]", "[========  ]", "[========= ]", "[==========]"}
		i := 0
		for {
			select {
			case <-doneCCH:
				fmt.Print("\r                    \r")
				return
			default:
				fmt.Printf("\r%s", chars[i%len(chars)])
				i++
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()
	cch := graph.PreprocessCCH(g)
	cch.Customize()
	doneCCH <- true
	time.Sleep(100 * time.Millisecond)
	fmt.Println("CCH ready!")
	//Grab any two node IDs from the graph to test with.
	j := 0
	for j < 10 {
		var startID, endID int64
		i := 0
		randomOffset := rand.Intn(len(g.Nodes) - 100)
		for id := range g.Nodes {
			if i == randomOffset {
				startID = id
			}
			if i == randomOffset+100 {
				endID = id
			}
			i++
			if i > randomOffset+100 {
				break
			}
		}
		fmt.Printf("Starting Dijkstra:\n")
		fmt.Printf("Finding path from %d to %d...\n", startID, endID)
		start_Dijkstra := time.Now()
		path_Dijkstra, time_Dijkstra, err := graph.Dijkstra(g, startID, endID)
		elapsed_Dijkstra := time.Since(start_Dijkstra)
		if err != nil {
			fmt.Printf("No path found, skipping...\n")
			j++
			continue
		}
		fmt.Printf("Path found. %d nodes, %.2f hours in %v\n", len(path_Dijkstra), time_Dijkstra, elapsed_Dijkstra)
		fmt.Printf("Starting Bidirectional Dijkstra:\n")
		fmt.Printf("Finding path from %d to %d...\n", startID, endID)
		start_BidirectionalDijkstra := time.Now()
		path_BidirectionalDijkstra, time_BidirectionalDijkstra, err := graph.BidirectionalDijkstra(g, startID, endID)
		elapsed_BidirectionalDijkstra := time.Since(start_BidirectionalDijkstra)
		if err != nil {
			fmt.Printf("No path found, skipping...\n")
			j++
			continue
		}
		fmt.Printf("Path found. %d nodes, %.2f hours in %v\n", len(path_BidirectionalDijkstra), time_BidirectionalDijkstra, elapsed_BidirectionalDijkstra)
		fmt.Printf("Starting A*:\n")
		fmt.Printf("Finding path from %d to %d...\n", startID, endID)
		start_A := time.Now()
		path_A, time_A, err := graph.AStar(g, startID, endID)
		elapsed_A := time.Since(start_A)
		if err != nil {
			fmt.Printf("No path found, skipping...\n")
			j++
			continue
		}
		fmt.Printf("Path found. %d nodes, %.2f hours in %v\n", len(path_A), time_A, elapsed_A)
		fmt.Printf("Starting New Bidirectional A*:\n")
		fmt.Printf("Finding path from %d to %d...\n", startID, endID)
		start_New_Bidirectional_A := time.Now()
		path_New_Bidirectional_A, time_New_Bidirectional_A, err := graph.NewBidirectionalAStar(g, startID, endID)
		elapsed_New_Bidirectional_A := time.Since(start_New_Bidirectional_A)
		if err != nil {
			fmt.Printf("No path found, skipping...\n")
			j++
			continue
		}
		fmt.Printf("Path found. %d nodes, %.2f hours in %v\n", len(path_New_Bidirectional_A), time_New_Bidirectional_A, elapsed_New_Bidirectional_A)
		fmt.Printf("Starting Contraction Hierarchies:\n")
		fmt.Printf("Finding path from %d to %d...\n", startID, endID)
		start_ch := time.Now()
		path_ch, time_ch, err := graph.CHQuery(ch, startID, endID)
		elapsed_ch := time.Since(start_ch)
		if err != nil {
			fmt.Printf("No path found, skipping...\n")
			j++
			continue
		}
		fmt.Printf("Path found. %d nodes, %.2f hours in %v\n", len(path_ch), time_ch, elapsed_ch)
		fmt.Printf("Starting CCH:\n")
		start_cch := time.Now()
		path_cch, time_cch, err := graph.CCHQuery(cch, startID, endID)
		elapsed_cch := time.Since(start_cch)
		if err != nil {
			fmt.Printf("No path found, skipping...\n")
			j++
			continue
		}
		fmt.Printf("Path found. %d nodes, %.2f hours in %v\n", len(path_cch), time_cch, elapsed_cch)
		fmt.Printf("-------------------------------------\n")
		j++
	}
}
