package graph

import (
	"container/heap"
	"errors"
	"fmt"
	"math"
)

// CHNode extends a regular node with a rank assigned during contraction.
type CHNode struct {
	Node
	Rank int
}

// CHEdge extends a regular edge with a flag indicating if it's a shortcut
// and which node it bypasses.
type CHEdge struct {
	Edge
	IsShortcut bool
	MiddleNode int64
}

// CHGraph is the augmented graph used for CH preprocessing and queries.
type CHGraph struct {
	Nodes   map[int64]CHNode
	Edges   map[int64][]CHEdge // Outgoing edges
	InEdges map[int64][]CHEdge // Incoming edges
	Ranks   map[int64]int      // Maps Node ID -> Contracted Rank
}

// NewCHGraph creates a CH graph from a regular graph, initializing both forward and backward edges.
func NewCHGraph(g *Graph) *CHGraph {
	ch := &CHGraph{
		Nodes:   make(map[int64]CHNode),
		Edges:   make(map[int64][]CHEdge),
		InEdges: make(map[int64][]CHEdge),
		Ranks:   make(map[int64]int),
	}
	for id, node := range g.Nodes {
		ch.Nodes[id] = CHNode{Node: node, Rank: -1}
	}
	for from, edges := range g.Edges {
		for _, edge := range edges {
			addOrUpdateEdge(ch, from, edge.To, edge.Time, false, -1)
		}
	}
	return ch
}

// Helper to get the time of a direct edge/shortcut between two nodes in the forward graph.
func getEdgeTime(ch *CHGraph, fromID int64, toID int64) (float64, bool) {
	for _, edge := range ch.Edges[fromID] {
		if edge.To == toID {
			return edge.Time, true
		}
	}
	return math.Inf(1), false
}

// Helper to add or update an edge in both the forward and backward graph.
func addOrUpdateEdge(ch *CHGraph, fromID int64, toID int64, time float64, isShortcut bool, middleNode int64) {
	//Update the forward edges (outgoing).
	found := false
	for i, edge := range ch.Edges[fromID] {
		if edge.To == toID {
			if time < edge.Time {
				ch.Edges[fromID][i].Time = time
				ch.Edges[fromID][i].IsShortcut = isShortcut
				ch.Edges[fromID][i].MiddleNode = middleNode
			}
			found = true
			break
		}
	}
	if !found {
		ch.Edges[fromID] = append(ch.Edges[fromID], CHEdge{
			Edge: Edge{
				To:   toID,
				Time: time,
			},
			IsShortcut: isShortcut,
			MiddleNode: middleNode,
		})
	}
	//Update the backward edges (incoming).
	foundIn := false
	for i, edge := range ch.InEdges[toID] {
		if edge.To == fromID {
			if time < edge.Time {
				ch.InEdges[toID][i].Time = time
				ch.InEdges[toID][i].IsShortcut = isShortcut
				ch.InEdges[toID][i].MiddleNode = middleNode
			}
			foundIn = true
			break
		}
	}
	if !foundIn {
		ch.InEdges[toID] = append(ch.InEdges[toID], CHEdge{
			Edge: Edge{
				To:   fromID,
				Time: time,
			},
			IsShortcut: isShortcut,
			MiddleNode: middleNode,
		})
	}
}

// witnessSearch checks if a path exists between fromID and toID in the uncontracted graph.
func witnessSearch(ch *CHGraph, fromID int64, toID int64, ignoredNode int64, maxTime float64, hopLimit int) bool {
	timeTo := make(map[int64]float64)
	timeTo[fromID] = 0
	hops := make(map[int64]int)
	hops[fromID] = 0
	pq := &PriorityQueue{{nodeID: fromID, time: 0}}
	heap.Init(pq)
	for pq.Len() > 0 {
		current := heap.Pop(pq).(*Item)
		if current.nodeID == toID {
			return current.time <= maxTime
		}
		if current.time > maxTime {
			return false
		}
		if hops[current.nodeID] >= hopLimit {
			continue
		}
		for _, edge := range ch.Edges[current.nodeID] {
			if edge.To == ignoredNode {
				continue
			}
			// Important: Skip already contracted nodes in the witness search
			if _, contracted := ch.Ranks[edge.To]; contracted {
				continue
			}
			newTime := timeTo[current.nodeID] + edge.Time
			existing, ok := timeTo[edge.To]
			if !ok || newTime < existing {
				timeTo[edge.To] = newTime
				hops[edge.To] = hops[current.nodeID] + 1
				heap.Push(pq, &Item{nodeID: edge.To, time: newTime})
			}
		}
	}
	return false
}

// contractNode contracts a single node, adding shortcuts between its uncontracted neighbors.
func contractNode(ch *CHGraph, nodeID int64, rank int, hopLimit int) int {
	shortcuts := 0
	inEdges := ch.InEdges[nodeID]
	outEdges := ch.Edges[nodeID]
	for _, inEdge := range inEdges {
		u := inEdge.To
		if _, contracted := ch.Ranks[u]; contracted {
			continue
		}
		for _, outEdge := range outEdges {
			w := outEdge.To
			if _, contracted := ch.Ranks[w]; contracted {
				continue
			}
			if u == w {
				continue
			}
			shortcutTime := inEdge.Time + outEdge.Time
			//If an existing direct edge is already shorter, skip witness search.
			if existingTime, exists := getEdgeTime(ch, u, w); exists && existingTime <= shortcutTime {
				continue
			}
			witnessExists := witnessSearch(ch, u, w, nodeID, shortcutTime, hopLimit)
			if !witnessExists {
				addOrUpdateEdge(ch, u, w, shortcutTime, true, nodeID)
				shortcuts++
			}
		}
	}
	//Assign the rank to this node.
	ch.Ranks[nodeID] = rank
	if node, ok := ch.Nodes[nodeID]; ok {
		node.Rank = rank
		ch.Nodes[nodeID] = node
	}
	return shortcuts
}

// calculateEdgeDifference simulates contraction and computes the edge difference.
func calculateEdgeDifference(ch *CHGraph, nodeID int64, hopLimit int) int {
	shortcuts := 0
	inEdges := ch.InEdges[nodeID]
	outEdges := ch.Edges[nodeID]
	for _, inEdge := range inEdges {
		u := inEdge.To
		if _, contracted := ch.Ranks[u]; contracted {
			continue
		}
		for _, outEdge := range outEdges {
			w := outEdge.To
			if _, contracted := ch.Ranks[w]; contracted {
				continue
			}
			if u == w {
				continue
			}
			shortcutTime := inEdge.Time + outEdge.Time
			if existingTime, exists := getEdgeTime(ch, u, w); exists && existingTime <= shortcutTime {
				continue
			}
			witnessExists := witnessSearch(ch, u, w, nodeID, shortcutTime, hopLimit)
			if !witnessExists {
				shortcuts++
			}
		}
	}
	//Count uncontracted incoming and outgoing edges.
	edgesRemoved := 0
	for _, edge := range ch.InEdges[nodeID] {
		if _, contracted := ch.Ranks[edge.To]; !contracted {
			edgesRemoved++
		}
	}
	for _, edge := range ch.Edges[nodeID] {
		if _, contracted := ch.Ranks[edge.To]; !contracted {
			edgesRemoved++
		}
	}
	return shortcuts - edgesRemoved
}

// Preprocess builds the CH by contracting all nodes using lazy priority queue updates.
func Preprocess(g *Graph, progress func(float64)) *CHGraph {
	ch := NewCHGraph(g)
	hopLimit := 5
	numNodes := len(ch.Nodes)
	pq := &PriorityQueue{}
	heap.Init(pq)
	// Compute initial importance.
	for id := range ch.Nodes {
		edgeDiff := calculateEdgeDifference(ch, id, hopLimit)
		heap.Push(pq, &Item{nodeID: id, time: float64(edgeDiff)})
	}
	rank := 0
	for pq.Len() > 0 {
		item := heap.Pop(pq).(*Item)
		nodeID := item.nodeID
		// Recompute the edge difference.
		currentEdgeDiff := calculateEdgeDifference(ch, nodeID, hopLimit)
		// If it's no longer the minimum, push it back and continue.
		if pq.Len() > 0 && float64(currentEdgeDiff) > (*pq)[0].time {
			item.time = float64(currentEdgeDiff)
			heap.Push(pq, item)
			continue
		}
		contractNode(ch, nodeID, rank, hopLimit)
		rank++
		if progress != nil && numNodes > 0 {
			progress(float64(rank) / float64(numNodes))
		}
	}
	fmt.Printf("\nCH preprocessing complete. %d nodes contracted.\n", numNodes)
	return ch
}

// CHQuery runs a bidirectional upward search on the CH graph.
func CHQuery(ch *CHGraph, startID int64, endID int64) ([]int64, float64, error) {
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
		//Forward step.
		if forwardPQ.Len() > 0 {
			forwardCurrent := heap.Pop(forwardPQ).(*Item)
			u := forwardCurrent.nodeID
			if forwardCurrent.time >= bestTime {
				//Terminate the forward frontier early.
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
				for _, edge := range ch.Edges[u] {
					v := edge.To
					if ch.Ranks[v] <= ch.Ranks[u] {
						continue
					}
					newTime := forwardTime[u] + edge.Time
					existing, ok := forwardTime[v]
					if !ok || newTime < existing {
						forwardTime[v] = newTime
						forwardPrev[v] = u
						if bTime, reached := backwardTime[v]; reached {
							if newTime+bTime < bestTime {
								bestTime = newTime + bTime
								meetingNode = v
							}
						}
						heap.Push(forwardPQ, &Item{nodeID: v, time: newTime})
					}
				}
			}
		}
		//Backward step (moving backwards using InEdges).
		if backwardPQ.Len() > 0 {
			backwardCurrent := heap.Pop(backwardPQ).(*Item)
			v := backwardCurrent.nodeID
			if backwardCurrent.time >= bestTime {
				//Terminate the backward frontier early.
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
				for _, edge := range ch.InEdges[v] {
					w := edge.To
					if ch.Ranks[w] <= ch.Ranks[v] {
						continue
					}
					newTime := backwardTime[v] + edge.Time
					existing, ok := backwardTime[w]
					if !ok || newTime < existing {
						backwardTime[w] = newTime
						backwardPrev[w] = v
						if fTime, reached := forwardTime[w]; reached {
							if fTime+newTime < bestTime {
								bestTime = fTime + newTime
								meetingNode = w
							}
						}
						heap.Push(backwardPQ, &Item{nodeID: w, time: newTime})
					}
				}
			}
		}
	}
	if meetingNode == -1 || math.IsInf(bestTime, 1) {
		return nil, 0, errors.New("no path found")
	}
	path, err := unpackPath(ch, forwardPrev, backwardPrev, startID, endID, meetingNode)
	if err != nil {
		return nil, 0, err
	}
	return path, bestTime, nil
}

// unpackPath reconstructs the full path by recursively unpacking shortcuts.
func unpackPath(ch *CHGraph, forwardPrev map[int64]int64, backwardPrev map[int64]int64, startID int64, endID int64, meetingNode int64) ([]int64, error) {
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
	backwardPath := []int64{}
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
		unpacked := unpackEdge(ch, rawPath[i], rawPath[i+1])
		fullPath = append(fullPath, unpacked...)
	}
	return fullPath, nil
}

// unpackEdge recursively unpacks a single edge, expanding shortcuts.
func unpackEdge(ch *CHGraph, fromID int64, toID int64) []int64 {
	for _, edge := range ch.Edges[fromID] {
		if edge.To == toID {
			if edge.IsShortcut {
				firstHalf := unpackEdge(ch, fromID, edge.MiddleNode)
				secondHalf := unpackEdge(ch, edge.MiddleNode, toID)
				return append(firstHalf, secondHalf...)
			}
			return []int64{toID}
		}
	}
	return []int64{toID}
}
