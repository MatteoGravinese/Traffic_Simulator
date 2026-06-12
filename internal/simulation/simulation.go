package simulation

import (
	"math"
	"math/rand"
	"sync"

	"github.com/MatteoGravinese/Traffic_Simulator/internal/graph"
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
	EdgePriorities map[int64]map[int64]int    // fromID -> toID -> priority
	EdgeLookup     map[int64]map[int64]graph.Edge // fromID -> toID -> edge (O(1) lookup)
	mu             sync.RWMutex
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
func NewSimState(g *graph.Graph, ch *graph.CHGraph, cch *graph.CCHGraph) *SimState {
	state := &SimState{
		G:              g,
		CH:             ch,
		CCH:            cch,
		Vehicles:       make([]*Vehicle, 0),
		Congestion:     make(map[int64]map[int64]float64),
		CongestionEMA:  make(map[int64]map[int64]float64),
		EdgePriorities: make(map[int64]map[int64]int),
		EdgeLookup:     make(map[int64]map[int64]graph.Edge),
	}
	if g != nil {
		state.NodeIDs, state.EdgePriorities, state.EdgeLookup = buildGraphIndexes(g)
	}
	return state
}

func buildGraphIndexes(g *graph.Graph) ([]int64, map[int64]map[int64]int, map[int64]map[int64]graph.Edge) {
	nodeIDs := make([]int64, 0, len(g.Nodes))
	for id := range g.Nodes {
		nodeIDs = append(nodeIDs, id)
	}
	edgePriorities := make(map[int64]map[int64]int)
	edgeLookup := make(map[int64]map[int64]graph.Edge)
	for fromID, edges := range g.Edges {
		edgePriorities[fromID] = make(map[int64]int)
		edgeLookup[fromID] = make(map[int64]graph.Edge)
		for _, edge := range edges {
			priority := 6
			if p, ok := HighwayPriority[edge.Highway]; ok {
				priority = p
			}
			edgePriorities[fromID][edge.To] = priority
			edgeLookup[fromID][edge.To] = edge
		}
	}
	return nodeIDs, edgePriorities, edgeLookup
}

// UpdateGraph thread-safely swaps the road network graph and hierarchy structures.
func (s *SimState) UpdateGraph(g *graph.Graph, ch *graph.CHGraph, cch *graph.CCHGraph, progress func(float64)) {
	s.mu.Lock()
	s.G = g
	s.CH = ch
	s.CCH = cch
	s.Vehicles = make([]*Vehicle, 0)
	s.Congestion = make(map[int64]map[int64]float64)
	s.CongestionEMA = make(map[int64]map[int64]float64)
	nodeIDs, edgePriorities, edgeLookup := buildGraphIndexes(g)
	s.NodeIDs = nodeIDs
	s.EdgePriorities = edgePriorities
	s.EdgeLookup = edgeLookup
	s.mu.Unlock()
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
		// Route on smoothed EMA to avoid oscillation from raw per-tick density spikes.
		state.CCH.Customize(state.CongestionEMA)
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
	finishedIndices := make([]int, 0, len(vehicles)/10)
	for i, vehicle := range vehicles {
		if len(vehicle.Path) < 2 || vehicle.CurrentIndex >= len(vehicle.Path)-1 {
			finishedIndices = append(finishedIndices, i)
			continue
		}
		timeRemaining := tickInterval
		for timeRemaining > 0 && vehicle.CurrentIndex < len(vehicle.Path)-1 {
			curr_node := vehicle.Path[vehicle.CurrentIndex]
			next_node := vehicle.Path[vehicle.CurrentIndex+1]
			curr_edge, foundEdge := lookupEdge(state, curr_node, next_node)
			if !foundEdge {
				vehicle.CurrentIndex = len(vehicle.Path) - 1
				break
			}
			speed := effectiveSpeed(curr_edge.Speed, curr_node, next_node, state.Congestion)
			if speed <= 0 {
				break
			}
			distToNext := curr_edge.Distance - vehicle.DistanceTravelled
			distThisTick := speed * timeRemaining / 3600
			if distThisTick >= distToNext {
				timeRemaining -= distToNext * 3600 / speed
				vehicle.DistanceTravelled = 0
				vehicle.CurrentIndex++
				if vehicle.CurrentIndex >= len(vehicle.Path)-1 {
					break
				}
			} else {
				vehicle.DistanceTravelled += distThisTick
				timeRemaining = 0
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

// effectiveSpeed returns the congestion-adjusted speed for an edge.
// Uses the same 1x–4x travel-time curve as CCH routing: speed / (1 + 3*density).
func effectiveSpeed(baseSpeed float64, fromID, toID int64, congestion map[int64]map[int64]float64) float64 {
	density := 0.0
	if toMap, ok := congestion[fromID]; ok {
		if d, ok := toMap[toID]; ok {
			density = d
		}
	}
	return baseSpeed / (1.0 + 3.0*density)
}

func lookupEdge(state *SimState, fromID, toID int64) (graph.Edge, bool) {
	if toMap, ok := state.EdgeLookup[fromID]; ok {
		if edge, ok := toMap[toID]; ok {
			return edge, true
		}
	}
	return graph.Edge{}, false
}

func clearNestedFloatMap(m map[int64]map[int64]float64) {
	for fromID, toMap := range m {
		for toID := range toMap {
			delete(toMap, toID)
		}
		delete(m, fromID)
	}
}

func updateCongestion(state *SimState, vehicles []*Vehicle) {
	// 1. Clear the current snapshot map in place to avoid per-tick allocations.
	clearNestedFloatMap(state.Congestion)
	total_cars := make(map[int64]map[int64]int)
	for _, vehicle := range vehicles {
		if len(vehicle.Path) < 2 || vehicle.CurrentIndex >= len(vehicle.Path)-1 {
			continue
		}
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
			edge, ok := lookupEdge(state, node_from, node_to)
			if !ok {
				continue
			}
			street_length := edge.Distance
			lane_number := float64(edge.Lanes)
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
