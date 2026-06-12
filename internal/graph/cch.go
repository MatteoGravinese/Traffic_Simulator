package graph

import (
	"container/heap"
	"errors"
	"fmt"
	"math"
)

// Item represents a node in the priority queue.
type Item struct {
	nodeID int64
	time   float64
	index  int
}

// PriorityQueue implements heap.Interface.
type PriorityQueue []*Item

func (pq PriorityQueue) Len() int {
	return len(pq)
}

func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].time < pq[j].time
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	item := x.(*Item)
	item.index = len(*pq)
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	item := old[len(old)-1]
	*pq = old[:len(old)-1]
	return item
}

// CCHEdge represents an edge (original or shortcut) in the CCH graph.
type CCHEdge struct {
	From             int64
	To               int64
	OriginalTime     float64 // Free-flow travel time in hours (Distance / Speed).
	IsShortcut       bool
	MiddleNode       int64
	CustomizedWeight float64
}

// CustomizationTask stores the static references needed to update a shortcut's weight.
// from the weights of two lower-ranked edges: u -> v and v -> w (where rank(v) < rank(u), rank(w)).
type CustomizationTask struct {
	InEdge   *CCHEdge
	OutEdge  *CCHEdge
	Shortcut *CCHEdge
	MidNode  int64
}

// CCHGraph is the static topology of the graph with precomputed customization tasks.
type CCHGraph struct {
	Nodes              map[int64]Node
	Edges              map[int64][]*CCHEdge //Outgoing edges.
	InEdges            map[int64][]*CCHEdge //Incoming edges.
	Ranks              map[int64]int        //Maps Node ID -> Contraction Rank.
	Order              []int64              //Nodes ordered by rank.
	CustomizationTasks []CustomizationTask
}

// NewCCHGraph initializes an empty CCHGraph.
func NewCCHGraph() *CCHGraph {
	return &CCHGraph{
		Nodes:              make(map[int64]Node),
		Edges:              make(map[int64][]*CCHEdge),
		InEdges:            make(map[int64][]*CCHEdge),
		Ranks:              make(map[int64]int),
		Order:              make([]int64, 0),
		CustomizationTasks: make([]CustomizationTask, 0),
	}
}

// findEdge returns the edge from -> to if it exists.
func (cch *CCHGraph) findEdge(from, to int64) *CCHEdge {
	for _, e := range cch.Edges[from] {
		if e.To == to {
			return e
		}
	}
	return nil
}

// getOrCreateEdge returns the edge from -> to, creating it if it doesn't exist.
func (cch *CCHGraph) getOrCreateEdge(from, to int64, isShortcut bool) *CCHEdge {
	if e := cch.findEdge(from, to); e != nil {
		return e
	}
	e := &CCHEdge{
		From:             from,
		To:               to,
		OriginalTime:     0,
		IsShortcut:       isShortcut,
		MiddleNode:       -1,
		CustomizedWeight: math.MaxFloat64,
	}
	cch.Edges[from] = append(cch.Edges[from], e)
	cch.InEdges[to] = append(cch.InEdges[to], e)
	return e
}

// PreprocessCCH builds the metric-independent CCH topology and precomputes customization tasks.
func PreprocessCCH(g *Graph, progress func(float64)) *CCHGraph {
	cch := NewCCHGraph()
	for id, node := range g.Nodes {
		cch.Nodes[id] = node
	}
	//1. Build the active neighbor lists for greedy degree contraction.
	activeIn := make(map[int64]map[int64]bool)
	activeOut := make(map[int64]map[int64]bool)
	activeNeighbors := make(map[int64]map[int64]bool)
	for id := range g.Nodes {
		activeIn[id] = make(map[int64]bool)
		activeOut[id] = make(map[int64]bool)
		activeNeighbors[id] = make(map[int64]bool)
	}
	for from, edges := range g.Edges {
		for _, edge := range edges {
			to := edge.To
			if from == to {
				continue
			}
			activeOut[from][to] = true
			activeIn[to][from] = true
			activeNeighbors[from][to] = true
			activeNeighbors[to][from] = true
		}
	}
	//Initialize Priority Queue based on degree.
	pq := &PriorityQueue{}
	heap.Init(pq)
	for id := range g.Nodes {
		heap.Push(pq, &Item{nodeID: id, time: float64(len(activeNeighbors[id]))})
	}
	contracted := make(map[int64]bool)
	ranks := make(map[int64]int)
	order := make([]int64, len(g.Nodes))
	rank := 0
	type ShortcutToAdd struct {
		From int64
		To   int64
	}
	shortcuts := make([]ShortcutToAdd, 0)
	numNodes := len(g.Nodes)
	//Contraction loop.
	for pq.Len() > 0 {
		item := heap.Pop(pq).(*Item)
		v := item.nodeID
		if contracted[v] {
			continue
		}
		//Lazy degree update check.
		currDegree := len(activeNeighbors[v])
		if pq.Len() > 0 && float64(currDegree) > (*pq)[0].time {
			item.time = float64(currDegree)
			heap.Push(pq, item)
			continue
		}
		//Contract v.
		ranks[v] = rank
		order[rank] = v
		contracted[v] = true
		//Add shortcuts between all uncontracted neighbors of v.
		for u := range activeIn[v] {
			if contracted[u] {
				continue
			}
			for w := range activeOut[v] {
				if contracted[w] {
					continue
				}
				if u == w {
					continue
				}
				//If u -> w does not exist, add it.
				if !activeOut[u][w] {
					shortcuts = append(shortcuts, ShortcutToAdd{From: u, To: w})
					activeOut[u][w] = true
					activeIn[w][u] = true
					if !activeNeighbors[u][w] {
						activeNeighbors[u][w] = true
						activeNeighbors[w][u] = true
						//Push back to update priorities in PQ.
						heap.Push(pq, &Item{nodeID: u, time: float64(len(activeNeighbors[u]))})
						heap.Push(pq, &Item{nodeID: w, time: float64(len(activeNeighbors[w]))})
					}
				}
			}
		}
		rank++
		if progress != nil && numNodes > 0 {
			progress(float64(rank) / float64(numNodes))
		}
	}
	cch.Ranks = ranks
	cch.Order = order
	//2. Add original edges to CCHGraph.
	for from, edges := range g.Edges {
		for _, edge := range edges {
			e := cch.getOrCreateEdge(from, edge.To, false)
			e.OriginalTime = edge.Time
		}
	}
	//3. Add shortcut edges to CCHGraph.
	for _, sc := range shortcuts {
		cch.getOrCreateEdge(sc.From, sc.To, true)
	}
	//4. Precompute customization tasks (flat list of updates).
	for _, v := range order {
		for _, inEdge := range cch.InEdges[v] {
			u := inEdge.From
			if ranks[u] <= ranks[v] {
				continue
			}
			for _, outEdge := range cch.Edges[v] {
				w := outEdge.To
				if ranks[w] <= ranks[v] {
					continue
				}
				if u == w {
					continue
				}
				shortcut := cch.findEdge(u, w)
				if shortcut != nil {
					cch.CustomizationTasks = append(cch.CustomizationTasks, CustomizationTask{
						InEdge:   inEdge,
						OutEdge:  outEdge,
						Shortcut: shortcut,
						MidNode:  v,
					})
				}
			}
		}
	}
	fmt.Printf("\nCCH Preprocessing complete: %d nodes, %d original/shortcut edges, %d customization tasks.\n",
		len(cch.Nodes), len(cch.Ranks), len(cch.CustomizationTasks))

	return cch
}

// Customize evaluates original edge weights, adjusts them for traffic congestion,
// and propagates them bottom-up through the shortcuts.
func (cch *CCHGraph) Customize(congestion map[int64]map[int64]float64) {
	for _, edges := range cch.Edges {
		for _, e := range edges {
			if !e.IsShortcut {
				weight := e.OriginalTime
				if toMap, exists := congestion[e.From]; exists {
					if density, hasDensity := toMap[e.To]; hasDensity && density > 0 {
						// density is already normalized to [0, 1] where:
						// 0.0 - 0.18 = free flow (< 12 vehicles/mile/lane)
						// 0.18 - 0.45 = stable flow (12-30 vehicles/mile/lane)
						// 0.45 - 1.0  = congested/breakdown (30-67 vehicles/mile/lane)
						// At full congestion (density = 1.0), travel time is 4x the original.
						// At free flow (density = 0.0), travel time is unchanged.
						weight = e.OriginalTime * (1.0 + 3.0*density)
					}
				}
				e.CustomizedWeight = weight
			} else {
				e.CustomizedWeight = math.MaxFloat64
				e.MiddleNode = -1
			}
		}
	}
	//Propagate updated weights in bottom-up rank order.
	for _, task := range cch.CustomizationTasks {
		if task.InEdge.CustomizedWeight == math.MaxFloat64 || task.OutEdge.CustomizedWeight == math.MaxFloat64 {
			continue
		}
		newWeight := task.InEdge.CustomizedWeight + task.OutEdge.CustomizedWeight
		if newWeight < task.Shortcut.CustomizedWeight {
			task.Shortcut.CustomizedWeight = newWeight
			task.Shortcut.MiddleNode = task.MidNode
		}
	}
}

// CCHQuery finds the shortest path between startID and endID using CCH ranks.
func CCHQuery(cch *CCHGraph, startID, endID int64) ([]int64, float64, error) {
	// Ensure forward search starts from the lower‑rank node.
	swapped := false // Do not swap start/end based on rank; pathfinding must respect original direction
	if startID == endID {
		return []int64{startID}, 0, nil
	}
	forwardTime := make(map[int64]float64)
	backwardTime := make(map[int64]float64)
	forwardPrev := make(map[int64]int64)
	backwardPrev := make(map[int64]int64)
	forwardVisited := make(map[int64]bool)
	backwardVisited := make(map[int64]bool)
	forwardTime[startID] = 0
	backwardTime[endID] = 0
	forwardPQ := &PriorityQueue{{nodeID: startID, time: 0}}
	backwardPQ := &PriorityQueue{{nodeID: endID, time: 0}}
	heap.Init(forwardPQ)
	heap.Init(backwardPQ)
	bestTime := math.Inf(1)
	meetingNode := int64(-1)
	for forwardPQ.Len() > 0 || backwardPQ.Len() > 0 {
		// Forward step
		if forwardPQ.Len() > 0 {
			curr := heap.Pop(forwardPQ).(*Item)
			u := curr.nodeID
			if curr.time >= bestTime {
				forwardPQ = &PriorityQueue{}
			} else if !forwardVisited[u] {
				forwardVisited[u] = true
				if backwardVisited[u] {
					combined := forwardTime[u] + backwardTime[u]
					if combined < bestTime {
						bestTime = combined
						meetingNode = u
					}
				}
				for _, edge := range cch.Edges[u] {
					v := edge.To
					if cch.Ranks[v] <= cch.Ranks[u] {
						continue
					}
					weight := edge.CustomizedWeight
					if weight == math.MaxFloat64 {
						weight = edge.OriginalTime
					}
					if weight == math.MaxFloat64 {
						continue
					}
					newTime := forwardTime[u] + weight
					if existing, ok := forwardTime[v]; !ok || newTime < existing {
						forwardTime[v] = newTime
						forwardPrev[v] = u
						if bTime, reached := backwardTime[v]; reached && newTime+bTime < bestTime {
							bestTime = newTime + bTime
							meetingNode = v
						}
						heap.Push(forwardPQ, &Item{nodeID: v, time: newTime})
					}
				}
			}
		}
		// Backward step
		if backwardPQ.Len() > 0 {
			curr := heap.Pop(backwardPQ).(*Item)
			v := curr.nodeID
			if curr.time >= bestTime {
				backwardPQ = &PriorityQueue{}
			} else if !backwardVisited[v] {
				backwardVisited[v] = true
				if forwardVisited[v] {
					combined := forwardTime[v] + backwardTime[v]
					if combined < bestTime {
						bestTime = combined
						meetingNode = v
					}
				}
				for _, edge := range cch.InEdges[v] {
					u := edge.From
					if cch.Ranks[u] <= cch.Ranks[v] {
						continue
					}
					weight := edge.CustomizedWeight
					if weight == math.MaxFloat64 {
						weight = edge.OriginalTime
					}
					if weight == math.MaxFloat64 {
						continue
					}
					newTime := backwardTime[v] + weight
					if existing, ok := backwardTime[u]; !ok || newTime < existing {
						backwardTime[u] = newTime
						backwardPrev[u] = v
						if fTime, reached := forwardTime[u]; reached && fTime+newTime < bestTime {
							bestTime = fTime + newTime
							meetingNode = u
						}
						heap.Push(backwardPQ, &Item{nodeID: u, time: newTime})
					}
				}
			}
		}
	}
	if meetingNode == -1 || math.IsInf(bestTime, 1) {
		return nil, 0, errors.New("no path found")
	}
	path, err := CCHunpackPath(cch, forwardPrev, backwardPrev, startID, endID, meetingNode)
	if err != nil {
		return nil, 0, err
	}
	if swapped {
		// Reverse the path to match the original request direction.
		for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
			path[i], path[j] = path[j], path[i]
		}
	}
	return path, bestTime, nil
}

// unpackPath reconstructs the original path nodes.
func CCHunpackPath(cch *CCHGraph, forwardPrev, backwardPrev map[int64]int64, startID, endID, meetingNode int64) ([]int64, error) {
	reverseForward := make([]int64, 0, 16)
	current := meetingNode
	for current != startID {
		reverseForward = append(reverseForward, current)
		parent, ok := forwardPrev[current]
		if !ok {
			return nil, fmt.Errorf("no forward parent for node %d", current)
		}
		current = parent
	}
	forwardPath := make([]int64, len(reverseForward)+1)
	forwardPath[0] = startID
	for i, node := range reverseForward {
		forwardPath[len(reverseForward)-i] = node
	}
	backwardPath := make([]int64, 0)
	current = meetingNode
	for current != endID {
		parent, ok := backwardPrev[current]
		if !ok {
			return nil, fmt.Errorf("no backward parent for node %d", current)
		}
		current = parent
		backwardPath = append(backwardPath, current)
	}
	rawPath := append(forwardPath, backwardPath...)
	fullPath := []int64{rawPath[0]}
	for i := 0; i < len(rawPath)-1; i++ {
		unpacked := CCHunpackEdge(cch, rawPath[i], rawPath[i+1])
		fullPath = append(fullPath, unpacked...)
	}
	return fullPath, nil
}

// unpackEdge recursively unpacks a customized shortcut edge into original edges.
func CCHunpackEdge(cch *CCHGraph, fromID, toID int64) []int64 {
	edge := cch.findEdge(fromID, toID)
	if edge == nil {
		return []int64{toID}
	}
	if edge.IsShortcut {
		if edge.MiddleNode == -1 {
			//Fallback: if customization didn't set a middle node, return destination.
			return []int64{toID}
		}
		firstHalf := CCHunpackEdge(cch, fromID, edge.MiddleNode)
		secondHalf := CCHunpackEdge(cch, edge.MiddleNode, toID)
		return append(firstHalf, secondHalf...)
	}
	return []int64{toID}
}
