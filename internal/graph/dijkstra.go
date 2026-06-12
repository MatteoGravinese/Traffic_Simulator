package graph

import (
	"container/heap"
	"errors"
)

// Dijkstra finds the shortest path between two nodes.
func Dijkstra(g *Graph, startID int64, endID int64) ([]int64, float64, error) {
	//Tracks the shortest known time to every node.
	times := make(map[int64]float64)
	times[startID] = 0
	//Tracks how we got to each node so reconstructPath can use it later.
	previous := make(map[int64]int64)
	//Tracks visited nodes.
	visited := make(map[int64]bool)
	//Initializes the priority queue with the start node.
	pq := &PriorityQueue{{nodeID: startID, time: 0}}
	heap.Init(pq)
	for pq.Len() > 0 {
		//Pops the node with the smallest time.
		current := heap.Pop(pq).(*Item)
		//Skips the node if has already been visited.
		if visited[current.nodeID] {
			continue
		}
		visited[current.nodeID] = true
		//Checks if we reached the destination.
		if current.nodeID == endID {
			return reconstructPath(previous, startID, endID, times[endID])
		}
		//Otherwise, find the nearest unexplored neighbor.
		for _, edge := range g.Edges[current.nodeID] {
			if visited[edge.To] {
				continue
			}
			newDist := times[current.nodeID] + edge.Time
			existing, ok := times[edge.To]
			if !ok || newDist < existing {
				times[edge.To] = newDist
				previous[edge.To] = current.nodeID
				heap.Push(pq, &Item{nodeID: edge.To, time: newDist})
			}
		}
	}
	return nil, 0, errors.New("No path found.")
}

// reconstructPath walks backwards through previous map to build the path.
func reconstructPath(previous map[int64]int64, startID int64, endID int64, totalDist float64) ([]int64, float64, error) {
	path := []int64{}
	current := endID
	for current != startID {
		path = append([]int64{current}, path...)
		current = previous[current]
	}
	path = append([]int64{startID}, path...)
	return path, totalDist, nil
}
