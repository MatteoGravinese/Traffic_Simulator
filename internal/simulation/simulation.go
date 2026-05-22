package simulation

import (
	"math/rand"
	"sync"

	"github.com/MatteoGravinese/Traffic_Simulator/internal/graph"
	"github.com/redis/go-redis/v9"
)

type Algorithm int

const (
	AlgoCH Algorithm = iota
	AlgoDijkstra
	AlgoBidirectionalDijkstra
	AlgoAStar
	AlgoNBAStar
)

type Vehicle struct {
	ID                int
	StartID           int64
	EndID             int64
	Path              []int64
	CurrentIndex      int
	DistanceTravelled float64
	Algorithm         Algorithm
}

// SimState holds all shared simulation state.
type SimState struct {
	G          *graph.Graph
	CH         *graph.CHGraph
	Vehicles   []*Vehicle
	Congestion map[int64]map[int64]float64 //edge congestion: from -> to -> count.
	mu         sync.Mutex
	RDB        *redis.Client
	NodeIDs    []int64 //cached list of all node IDs for random selection.
}

// NewSimState creates a new simulation state.
func NewSimState(g *graph.Graph, ch *graph.CHGraph, rdb *redis.Client) *SimState {
	nodeIDs := make([]int64, 0, len(g.Nodes))
	for id := range g.Nodes {
		nodeIDs = append(nodeIDs, id)
	}
	return &SimState{
		G:          g,
		CH:         ch,
		Vehicles:   make([]*Vehicle, 0),
		Congestion: make(map[int64]map[int64]float64),
		RDB:        rdb,
		NodeIDs:    nodeIDs,
	}
}

func spawnVehicle(state *SimState, id int, algorithm Algorithm) *Vehicle {
	var path []int64
	var err error
	var startID int64
	var endID int64
	//Keep trying new start and end nodes until a valid path is found.
	for {
		startID = state.NodeIDs[rand.Intn(len(state.NodeIDs))]
		endID = state.NodeIDs[rand.Intn(len(state.NodeIDs))]
		for endID == startID {
			endID = state.NodeIDs[rand.Intn(len(state.NodeIDs))]
		}
		path, _, err = calculateRoute(state, startID, endID, algorithm)
		if err == nil {
			break
		}
	}
	return &Vehicle{
		ID:                id,
		StartID:           startID,
		EndID:             endID,
		Path:              path,
		CurrentIndex:      0,
		DistanceTravelled: 0,
		Algorithm:         algorithm,
	}
}

func calculateRoute(state *SimState, startID int64, endID int64, algorithm Algorithm) ([]int64, float64, error) {
	if algorithm == AlgoDijkstra {
		return graph.Dijkstra(state.G, startID, endID)
	}
	if algorithm == AlgoBidirectionalDijkstra {
		return graph.BidirectionalDijkstra(state.G, startID, endID)
	}
	if algorithm == AlgoAStar {
		return graph.AStar(state.G, startID, endID)
	}
	if algorithm == AlgoNBAStar {
		return graph.NewBidirectionalAStar(state.G, startID, endID)
	}
	return graph.CHQuery(state.CH, startID, endID)
}

func tick(state *SimState, vehicles []*Vehicle, tickInterval float64) {
	//Iterate through all vehicles
	for i, vehicle := range vehicles {
		curr_node := vehicle.Path[vehicle.CurrentIndex]
		next_node := vehicle.Path[vehicle.CurrentIndex+1]
		var firstEdge graph.Edge
		//Find the destination node.
		for _, edge := range state.G.Edges[curr_node] {
			if edge.To == next_node {
				firstEdge = edge
				break
			}
		}
		//Distance travelled
		remaining := tickInterval * firstEdge.Speed / 3600
		//If we pass through an intersection, see how far down we go.
		//If we go past more than one intersection, see how far we'll end up overall.
		for remaining > 0 && vehicle.CurrentIndex < len(vehicle.Path)-1 {
			curr_node := vehicle.Path[vehicle.CurrentIndex]
			next_node := vehicle.Path[vehicle.CurrentIndex+1]
			var curr_edge graph.Edge
			for _, edge := range state.G.Edges[curr_node] {
				if edge.To == next_node {
					curr_edge = edge
					break
				}
			}
			distToNext := curr_edge.Distance - vehicle.DistanceTravelled
			if remaining >= distToNext {
				remaining -= distToNext
				vehicle.DistanceTravelled = 0
				vehicle.CurrentIndex++
				//If the vehicle has reached its destination, spawn a new one in.
				if vehicle.CurrentIndex >= len(vehicle.Path)-1 {
					vehicles[i] = spawnVehicle(state, vehicle.ID, vehicle.Algorithm)
					break
				}
			} else {
				vehicle.DistanceTravelled += remaining
				remaining = 0
			}
		}
	}
	updateCongestion(state, vehicles)
}

func updateCongestion(state *SimState, vehicles []*Vehicle) {
	total_cars := make(map[int64]map[int64]int)
	for _, vehicle := range vehicles {
		curr_node := vehicle.Path[vehicle.CurrentIndex]
		next_node := vehicle.Path[vehicle.CurrentIndex+1]
		if total_cars[curr_node] == nil {
			total_cars[curr_node] = make(map[int64]int)
		}
		total_cars[curr_node][next_node]++
	}
	for node_from, _ := range total_cars {
		for node_to, cars := range total_cars[node_from] {
			var street_length float64
			var lane_number float64
			for _, edge := range state.G.Edges[node_from] {
				if edge.To == node_to {
					street_length = edge.Distance
					lane_number = float64(edge.Lanes)
					break
				}
			}
			if state.Congestion[node_from] == nil {
				state.Congestion[node_from] = make(map[int64]float64)
			}
			state.Congestion[node_from][node_to] = float64(cars) / (street_length * lane_number)
		}
	}
}
