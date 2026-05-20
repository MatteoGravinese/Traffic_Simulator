package graph

import (
	"container/heap"
	"errors"
	"math"
)

func BidirectionalDijkstra(g *Graph, startID int64, endID int64) ([]int64, float64, error) {
	//Forward search data.
	forwardDist := make(map[int64]float64)
	forwardPrev := make(map[int64]int64)
	forwardVisited := make(map[int64]bool)
	//Backward search data.
	backwardDist := make(map[int64]float64)
	backwardPrev := make(map[int64]int64)
	backwardVisited := make(map[int64]bool)
	forwardDist[startID] = 0
	backwardDist[endID] = 0
	//Initialize priority queues.
	forwardPQ := &PriorityQueue{{nodeID: startID, distance: 0}}
	backwardPQ := &PriorityQueue{{nodeID: endID, distance: 0}}
	heap.Init(forwardPQ)
	heap.Init(backwardPQ)
	bestDist := math.Inf(1)
	meetingNode := int64(-1)
	for forwardPQ.Len() > 0 && backwardPQ.Len() > 0 {
		//Check stopping condition.
		forwardTop := (*forwardPQ)[0].distance
		backwardTop := (*backwardPQ)[0].distance
		if forwardTop+backwardTop >= bestDist {
			break
		}
		//Forward step.
		forwardCurrent := heap.Pop(forwardPQ).(*Item)
		if !forwardVisited[forwardCurrent.nodeID] {
			forwardVisited[forwardCurrent.nodeID] = true
			//Check if backward search has visited this node.
			if backwardVisited[forwardCurrent.nodeID] {
				combined := forwardDist[forwardCurrent.nodeID] + backwardDist[forwardCurrent.nodeID]
				if combined < bestDist {
					bestDist = combined
					meetingNode = forwardCurrent.nodeID
				}
			}
			for _, edge := range g.Edges[forwardCurrent.nodeID] {
				if forwardVisited[edge.To] {
					continue
				}
				newDist := forwardDist[forwardCurrent.nodeID] + edge.Distance
				existing, ok := forwardDist[edge.To]
				if !ok || newDist < existing {
					forwardDist[edge.To] = newDist
					forwardPrev[edge.To] = forwardCurrent.nodeID
					heap.Push(forwardPQ, &Item{nodeID: edge.To, distance: newDist})
				}
			}
		}
		//Backward step.
		backwardCurrent := heap.Pop(backwardPQ).(*Item)
		if !backwardVisited[backwardCurrent.nodeID] {
			backwardVisited[backwardCurrent.nodeID] = true
			//Check if forward search has visited this node.
			if forwardVisited[backwardCurrent.nodeID] {
				combined := forwardDist[backwardCurrent.nodeID] + backwardDist[backwardCurrent.nodeID]
				if combined < bestDist {
					bestDist = combined
					meetingNode = backwardCurrent.nodeID
				}
			}
			for _, edge := range g.Edges[backwardCurrent.nodeID] {
				if backwardVisited[edge.To] {
					continue
				}
				newDist := backwardDist[backwardCurrent.nodeID] + edge.Distance
				existing, ok := backwardDist[edge.To]
				if !ok || newDist < existing {
					backwardDist[edge.To] = newDist
					backwardPrev[edge.To] = backwardCurrent.nodeID
					heap.Push(backwardPQ, &Item{nodeID: edge.To, distance: newDist})
				}
			}
		}
	}
	if meetingNode == -1 {
		return nil, 0, errors.New("no path found")
	}
	//Reconstruct path from both directions.
	path := []int64{}
	//Forward path: start to meeting node.
	current := meetingNode
	for current != startID {
		path = append([]int64{current}, path...)
		current = forwardPrev[current]
	}
	path = append([]int64{startID}, path...)
	//Backward path: meeting node to end.
	current = backwardPrev[meetingNode]
	for current != endID {
		path = append(path, current)
		current = backwardPrev[current]
	}
	path = append(path, endID)
	return path, bestDist, nil
}
