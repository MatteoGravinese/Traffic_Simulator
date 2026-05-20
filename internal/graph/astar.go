package graph

import (
	"container/heap"
	"errors"
	"math"
)

func AStar(g *Graph, startID int64, endID int64) ([]int64, float64, error) {
	end, ok := g.Nodes[endID]
	if !ok {
		return nil, 0, errors.New("end node not found")
	}
	distances := make(map[int64]float64)
	for id := range g.Nodes {
		distances[id] = math.Inf(1)
	}
	distances[startID] = 0
	previous := make(map[int64]int64)
	visited := make(map[int64]bool)
	pq := &PriorityQueue{{nodeID: startID, distance: 0}}
	heap.Init(pq)
	for pq.Len() > 0 {
		current := heap.Pop(pq).(*Item)
		if visited[current.nodeID] {
			continue
		}
		visited[current.nodeID] = true
		if current.nodeID == endID {
			return reconstructPath(previous, startID, endID, distances[endID])
		}
		currentNode, ok := g.Nodes[current.nodeID]
		if !ok {
			continue
		}
		for _, edge := range g.Edges[current.nodeID] {
			if visited[edge.To] {
				continue
			}
			neighbor, ok := g.Nodes[edge.To]
			if !ok {
				continue
			}
			newDist := distances[current.nodeID] + edge.Distance
			if newDist < distances[edge.To] {
				distances[edge.To] = newDist
				previous[edge.To] = current.nodeID
				//Heuristic calculation.
				h := haversine(currentNode.Lat, currentNode.Lon, end.Lat, end.Lon)
				heap.Push(pq, &Item{nodeID: neighbor.ID, distance: newDist + h})
			}
		}
	}
	return nil, 0, errors.New("no path found")
}
