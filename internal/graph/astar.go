package graph

import (
	"container/heap"
	"errors"
	"fmt"
)

func AStar(g *Graph, startID int64, endID int64) ([]int64, float64, error) {
	end, ok := g.Nodes[endID]
	if !ok {
		return nil, 0, errors.New("end node not found")
	}
	distances := make(map[int64]float64)
	distances[startID] = 0
	previous := make(map[int64]int64)
	visited := make(map[int64]bool)
	pq := &PriorityQueue{{nodeID: startID, distance: 0}}
	heap.Init(pq)
	visits := 0
	for pq.Len() > 0 {
		current := heap.Pop(pq).(*Item)
		if visited[current.nodeID] {
			continue
		}
		visited[current.nodeID] = true
		visits = visits + 1
		if current.nodeID == endID {
			fmt.Printf("Total A* visits: %d\n", visits)
			return reconstructPath(previous, startID, endID, distances[endID])
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
			existing, ok := distances[edge.To]
			if !ok || newDist < existing {
				distances[edge.To] = newDist
				previous[edge.To] = current.nodeID
				//Heuristic calculation.
				h := haversine(neighbor.Lat, neighbor.Lon, end.Lat, end.Lon)
				heap.Push(pq, &Item{nodeID: neighbor.ID, distance: newDist + h})
			}
		}
	}
	return nil, 0, errors.New("no path found")
}
