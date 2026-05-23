package graph

import (
	"container/heap"
	"errors"
)

func AStar(g *Graph, startID int64, endID int64) ([]int64, float64, error) {
	end, ok := g.Nodes[endID]
	if !ok {
		return nil, 0, errors.New("end node not found")
	}
	times := make(map[int64]float64)
	times[startID] = 0
	previous := make(map[int64]int64)
	visited := make(map[int64]bool)
	pq := &PriorityQueue{{nodeID: startID, time: 0}}
	heap.Init(pq)
	for pq.Len() > 0 {
		current := heap.Pop(pq).(*Item)
		if visited[current.nodeID] {
			continue
		}
		visited[current.nodeID] = true
		if current.nodeID == endID {
			return reconstructPath(previous, startID, endID, times[endID])
		}
		for _, edge := range g.Edges[current.nodeID] {
			if visited[edge.To] {
				continue
			}
			neighbor, ok := g.Nodes[edge.To]
			if !ok {
				continue
			}
			newTime := times[current.nodeID] + edge.Time
			existing, ok := times[edge.To]
			if !ok || newTime < existing {
				times[edge.To] = newTime
				previous[edge.To] = current.nodeID
				//Heuristic calculation.
				h := heuristic(neighbor.Lat, neighbor.Lon, end.Lat, end.Lon, g.MaxSpeed)
				heap.Push(pq, &Item{nodeID: neighbor.ID, time: newTime + h})
			}
		}
	}
	return nil, 0, errors.New("no path found")
}
