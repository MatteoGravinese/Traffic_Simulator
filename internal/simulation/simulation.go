package simulation

import (
	"math"
	"math/rand"
	"sync"

	"github.com/MatteoGravinese/Traffic_Simulator/internal/graph"
	"github.com/redis/go-redis/v9"
)

type Vehicle struct {
	ID                int
	StartID           int64
	EndID             int64
	Path              []int64
	CurrentIndex      int
	DistanceTravelled float64
}

// SimState holds all shared simulation state.
type SimState struct {
	G              *graph.Graph
	CH             *graph.CHGraph
	CCH            *graph.CCHGraph
	Vehicles       []*Vehicle
	Congestion     map[int64]map[int64]float64 //edge congestion: from -> to -> count.
	CongestionEMA  map[int64]map[int64]float64
	EdgePriorities map[int64]map[int64]int // fromID -> toID -> priority
	mu             sync.RWMutex
	RDB            *redis.Client
	NodeIDs        []int64 //cached list of all node IDs for random selection.
	TrafficAware   bool
}

// HighwayPriority maps highway types to their rendering priority (0 = highest, 6 = lowest).
var HighwayPriority = map[string]int{
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

// RLock/RUnlock expose the read-side of the mutex for the HTTP server.
func (s *SimState) RLock()   { s.mu.RLock() }
func (s *SimState) RUnlock() { s.mu.RUnlock() }

// NewSimState creates a new simulation state.
func NewSimState(g *graph.Graph, ch *graph.CHGraph, cch *graph.CCHGraph, rdb *redis.Client) *SimState {
	state := &SimState{
		G:              g,
		CH:             ch,
		CCH:            cch,
		Vehicles:       make([]*Vehicle, 0),
		Congestion:     make(map[int64]map[int64]float64),
		CongestionEMA:  make(map[int64]map[int64]float64),
		EdgePriorities: make(map[int64]map[int64]int),
		RDB:            rdb,
	}
	if g != nil {
		nodeIDs := make([]int64, 0, len(g.Nodes))
		for id := range g.Nodes {
			nodeIDs = append(nodeIDs, id)
		}
		state.NodeIDs = nodeIDs
		for fromID, edges := range g.Edges {
			state.EdgePriorities[fromID] = make(map[int64]int)
			for _, edge := range edges {
				priority := 6
				if p, ok := HighwayPriority[edge.Highway]; ok {
					priority = p
				}
				state.EdgePriorities[fromID][edge.To] = priority
			}
		}
	}
	return state
}

// UpdateGraph thread-safely swaps the road network graph and hierarchy structures, and spawns new vehicles.
func (s *SimState) UpdateGraph(g *graph.Graph, ch *graph.CHGraph, cch *graph.CCHGraph, vehiclesPerEdge float64, progress func(float64)) {
	s.mu.Lock()
	s.G = g
	s.CH = ch
	s.CCH = cch
	s.Vehicles = make([]*Vehicle, 0)
	s.Congestion = make(map[int64]map[int64]float64)
	s.CongestionEMA = make(map[int64]map[int64]float64)
	s.EdgePriorities = make(map[int64]map[int64]int)
	nodeIDs := make([]int64, 0, len(g.Nodes))
	for id := range g.Nodes {
		nodeIDs = append(nodeIDs, id)
	}
	s.NodeIDs = nodeIDs
	for fromID, edges := range g.Edges {
		s.EdgePriorities[fromID] = make(map[int64]int)
		for _, edge := range edges {
			priority := 6
			if p, ok := HighwayPriority[edge.Highway]; ok {
				priority = p
			}
			s.EdgePriorities[fromID][edge.To] = priority
		}
	}
	s.mu.Unlock()
	// Spawn new vehicles on the new graph.
	edgeCount := 0
	for _, edges := range g.Edges {
		edgeCount += len(edges)
	}
	numVehicles := 0
	if vehiclesPerEdge > 0 {
		numVehicles = int(math.Round(float64(edgeCount) * vehiclesPerEdge))
		if numVehicles < 0 {
			numVehicles = 0
		}
	}
	spawned := make([]*Vehicle, 0, numVehicles)
	if len(nodeIDs) > 1 && numVehicles > 0 {
		updateInterval := numVehicles / 200
		if updateInterval < 1 {
			updateInterval = 1
		}
		if updateInterval > 500 {
			updateInterval = 500
		}
		for i := 0; i < numVehicles; i++ {
			spawned = append(spawned, SpawnVehicle(s, i))
			if progress != nil && ((i+1)%updateInterval == 0 || i == numVehicles-1) {
				progress(float64(i+1) / float64(numVehicles))
			}
		}
	}
	s.mu.Lock()
	s.Vehicles = spawned
	s.mu.Unlock()
	if progress != nil {
		progress(1.0)
	}
	if progress != nil {
		progress(1.0)
	}
}

// SpawnVehicles replaces the current vehicle fleet with a new set using an explicit vehicle count.
func (s *SimState) SpawnVehicles(vehicleCount int, progress func(float64)) {
	s.mu.RLock()
	g := s.G
	nodeIDs := append([]int64(nil), s.NodeIDs...)
	s.mu.RUnlock()
	if g == nil {
		if progress != nil {
			progress(1.0)
		}
		return
	}
	numVehicles := 0
	if vehicleCount > 0 {
		numVehicles = vehicleCount
	}
	spawned := make([]*Vehicle, 0, numVehicles)
	if len(nodeIDs) > 1 && numVehicles > 0 {
		updateInterval := numVehicles / 200
		if updateInterval < 1 {
			updateInterval = 1
		}
		if updateInterval > 500 {
			updateInterval = 500
		}
		for i := 0; i < numVehicles; i++ {
			spawned = append(spawned, SpawnVehicle(s, i))
			if progress != nil && ((i+1)%updateInterval == 0 || i == numVehicles-1) {
				progress(float64(i+1) / float64(numVehicles))
			}
		}
	}
	s.mu.Lock()
	s.Vehicles = spawned
	s.Congestion = make(map[int64]map[int64]float64)
	s.CongestionEMA = make(map[int64]map[int64]float64)
	s.mu.Unlock()
	if progress != nil {
		progress(1.0)
	}
}

func SpawnVehicle(state *SimState, id int) *Vehicle {
	var path []int64
	var err error
	var startID int64
	var endID int64
	if len(state.NodeIDs) < 2 {
		return &Vehicle{ID: id}
	}
	// Keep trying a limited number of routes to avoid blocking indefinitely.
	maxAttempts := 50
	for attempt := 0; attempt < maxAttempts; attempt++ {
		startID = state.NodeIDs[rand.Intn(len(state.NodeIDs))]
		endID = state.NodeIDs[rand.Intn(len(state.NodeIDs))]
		for endID == startID {
			endID = state.NodeIDs[rand.Intn(len(state.NodeIDs))]
		}
		path, _, err = calculateRoute(state, startID, endID)
		if err == nil && len(path) > 1 {
			return &Vehicle{
				ID:                id,
				StartID:           startID,
				EndID:             endID,
				Path:              path,
				CurrentIndex:      0,
				DistanceTravelled: 0,
			}
		}
	}
	return &Vehicle{ID: id}
}

func calculateRoute(state *SimState, startID int64, endID int64) ([]int64, float64, error) {
	if state.TrafficAware {
		return graph.CCHQuery(state.CCH, startID, endID)
	}
	return graph.CHQuery(state.CH, startID, endID)
}

func Tick(state *SimState, tickInterval float64, tickCount int) {
	state.mu.Lock()
	if state.G == nil || len(state.Vehicles) == 0 {
		state.mu.Unlock()
		return
	}
	gAtStart := state.G
	vehicles := state.Vehicles
	//Refresh the global congestion maps first so current data is available.
	updateCongestion(state, vehicles)
	// Update congestion in waves based on vehicle ID band.
	if state.TrafficAware {
		const maxVehiclesPerTick = 30
		bandCount := (len(vehicles) + maxVehiclesPerTick - 1) / maxVehiclesPerTick
		if bandCount < 1 {
			bandCount = 1
		}
		band := tickCount % bandCount
		state.CCH.Customize(state.Congestion)
		for i, vehicle := range vehicles {
			if len(vehicle.Path) < 2 || vehicle.CurrentIndex >= len(vehicle.Path)-1 {
				continue
			}
			if vehicle.ID%bandCount == band {
				newPath, _, err := graph.CCHQuery(state.CCH, vehicle.Path[vehicle.CurrentIndex], vehicle.EndID)
				if err == nil {
					vehicles[i].Path = append(vehicle.Path[:vehicle.CurrentIndex+1], newPath[1:]...)
				}
			}
		}
	}
	//Iterate through all vehicles to move them.
	var finishedIndices []int
	for i, vehicle := range vehicles {
		if len(vehicle.Path) < 2 || vehicle.CurrentIndex >= len(vehicle.Path)-1 {
			finishedIndices = append(finishedIndices, i)
			continue
		}
		curr_node := vehicle.Path[vehicle.CurrentIndex]
		next_node := vehicle.Path[vehicle.CurrentIndex+1]
		var firstEdge graph.Edge
		found := false
		// Find the destination node.
		for _, edge := range state.G.Edges[curr_node] {
			if edge.To == next_node {
				firstEdge = edge
				found = true
				break
			}
		}
		if !found {
			finishedIndices = append(finishedIndices, i)
			continue
		}
		// Distance travelled.
		remaining := tickInterval * firstEdge.Speed / 3600
		// If we pass through an intersection, see how far down the next road we go.
		// If we go past more than one intersection, see how far we'll end up overall.
		for remaining > 0 && vehicle.CurrentIndex < len(vehicle.Path)-1 {
			curr_node := vehicle.Path[vehicle.CurrentIndex]
			next_node := vehicle.Path[vehicle.CurrentIndex+1]
			var curr_edge graph.Edge
			foundEdge := false
			for _, edge := range state.G.Edges[curr_node] {
				if edge.To == next_node {
					curr_edge = edge
					foundEdge = true
					break
				}
			}
			if !foundEdge {
				vehicle.CurrentIndex = len(vehicle.Path) - 1
				break
			}
			distToNext := curr_edge.Distance - vehicle.DistanceTravelled
			if remaining >= distToNext {
				remaining -= distToNext
				vehicle.DistanceTravelled = 0
				vehicle.CurrentIndex++
				// If the vehicle has reached its destination, exit loop.
				if vehicle.CurrentIndex >= len(vehicle.Path)-1 {
					break
				}
			} else {
				vehicle.DistanceTravelled += remaining
				remaining = 0
			}
		}
		if vehicle.CurrentIndex >= len(vehicle.Path)-1 {
			finishedIndices = append(finishedIndices, i)
		}
	}
	state.mu.Unlock()
	// Spawn replacement vehicles outside of the lock (slow routing queries run here).
	if len(finishedIndices) > 0 {
		newVehicles := make([]*Vehicle, len(finishedIndices))
		for k, idx := range finishedIndices {
			newVehicles[k] = SpawnVehicle(state, vehicles[idx].ID)
		}
		// Re-acquire lock to assign new vehicles if the graph wasn't swapped in the meantime.
		state.mu.Lock()
		if state.G == gAtStart && len(state.Vehicles) == len(vehicles) {
			for k, idx := range finishedIndices {
				state.Vehicles[idx] = newVehicles[k]
			}
		}
		state.mu.Unlock()
	}
}

func updateCongestion(state *SimState, vehicles []*Vehicle) {
	// 1. Clear the current snapshot map completely to handle live positional counts
	state.Congestion = make(map[int64]map[int64]float64)
	total_cars := make(map[int64]map[int64]int)
	for _, vehicle := range vehicles {
		curr_node := vehicle.Path[vehicle.CurrentIndex]
		next_node := vehicle.Path[vehicle.CurrentIndex+1]
		if total_cars[curr_node] == nil {
			total_cars[curr_node] = make(map[int64]int)
		}
		total_cars[curr_node][next_node]++
	}
	// Define your EMA smoothing factor alpha (0.2 is a balanced starting default).
	const alpha = 0.2
	const maxDensity = 67.0
	// 2. Compute current raw snapshot density metrics.
	for node_from := range total_cars {
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
			// Calculate standard snapshot density.
			rawCongestion := math.Min(float64(cars)/(street_length*lane_number)/maxDensity, 1.0)
			state.Congestion[node_from][node_to] = rawCongestion
		}
	}
	// Update the persistent CongestionEMA map using historical state.
	// Note: We iterate over CongestionEMA first to cool down streets where vehicles left.
	for node_from, targets := range state.CongestionEMA {
		for node_to, prevEMA := range targets {
			currentRaw := 0.0
			if targetsMap, ok := state.Congestion[node_from]; ok {
				if val, exists := targetsMap[node_to]; exists {
					currentRaw = val
				}
			}
			// Apply EMA Formula.
			newEMA := (alpha * currentRaw) + ((1.0 - alpha) * prevEMA)
			// If the value drops near absolute zero, prune it to prevent memory leaks.
			if newEMA < 0.001 {
				delete(state.CongestionEMA[node_from], node_to)
			} else {
				state.CongestionEMA[node_from][node_to] = newEMA
			}
		}
		if len(state.CongestionEMA[node_from]) == 0 {
			delete(state.CongestionEMA, node_from)
		}
	}
	// Capture any brand new congestion segments that weren't in the historical EMA map yet.
	for node_from, targets := range state.Congestion {
		for node_to, currentRaw := range targets {
			if state.CongestionEMA[node_from] == nil {
				state.CongestionEMA[node_from] = make(map[int64]float64)
			}
			if _, exists := state.CongestionEMA[node_from][node_to]; !exists {
				// Initialize brand new traffic with its current raw value or a small alpha step.
				state.CongestionEMA[node_from][node_to] = alpha * currentRaw
			}
		}
	}
}

func (s *SimState) SetTrafficAware(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TrafficAware = enabled
}
