package graph

import (
	"container/heap"
	"errors"
	"math"
)

func BidirectionalDijkstra(g *Graph, startID int64, endID int64) ([]int64, float64, error) {
	//If start and end are the same, return early.
	if startID == endID {
		return []int64{startID}, 0, nil
	}
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
					//Check for path intersection during relaxation.
					if backDist, ok := backwardDist[edge.To]; ok {
						combined := newDist + backDist
						if combined < bestDist {
							bestDist = combined
							meetingNode = edge.To
						}
					}
				}
			}
		}
		//Backward step.
		backwardCurrent := heap.Pop(backwardPQ).(*Item)
		if !backwardVisited[backwardCurrent.nodeID] {
			backwardVisited[backwardCurrent.nodeID] = true
			//Note: If directed graph, you must traverse incoming edges instead of outgoing.
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
					//Check path intersection during relaxation.
					if fDist, ok := forwardDist[edge.To]; ok {
						combined := newDist + fDist
						if combined < bestDist {
							bestDist = combined
							meetingNode = edge.To
						}
					}
				}
			}
		}
	}
	if meetingNode == -1 {
		return nil, 0, errors.New("no path found")
	}
	//Reconstruct the path from both directions.
	var forwardPath []int64
	curr := meetingNode
	for {
		forwardPath = append(forwardPath, curr)
		if curr == startID {
			break
		}
		curr = forwardPrev[curr]
	}
	//Reverse forwardPath to get startID -> ... -> meetingNode.
	for i, j := 0, len(forwardPath)-1; i < j; i, j = i+1, j-1 {
		forwardPath[i], forwardPath[j] = forwardPath[j], forwardPath[i]
	}
	//Backward path: meeting node to end (excluding meetingNode itself).
	var backwardPath []int64
	curr = meetingNode
	for curr != endID {
		curr = backwardPrev[curr]
		backwardPath = append(backwardPath, curr)
	}
	path := append(forwardPath, backwardPath...)
	return path, bestDist, nil
}
