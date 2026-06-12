package main

import (
	"fmt"
	"time"

	"github.com/MatteoGravinese/Traffic_Simulator/internal/api"
	"github.com/MatteoGravinese/Traffic_Simulator/internal/simulation"
)

func main() {
	fmt.Println("Traffic Simulator starting...")

	// Initialize state with nil graph, CH, and CCH. The graph will be populated when the user selects a region.
	state := simulation.NewSimState(nil, nil, nil)

	fmt.Println("Starting web server on http://localhost:8080...")
	go api.StartServer(state)

	// Run the simulation tick loop.
	go func() {
		tickCount := 0
		for {
			simulation.Tick(state, 30.0, tickCount)
			tickCount++
			time.Sleep(100 * time.Millisecond)
		}
	}()

	select {}
}
