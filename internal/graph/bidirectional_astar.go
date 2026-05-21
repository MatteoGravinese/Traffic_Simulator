package graph

import (
	"container/heap"
	"errors"
	"fmt"
	"math"
)

func NewBidirectionalAStar(g *Graph, startID int64, endID int64) ([]int64, float64, error) {
	start, ok := g.Nodes[startID]
	if !ok {
		return nil, 0, errors.New("start node not found")
	}
	end, ok := g.Nodes[endID]
	if !ok {
		return nil, 0, errors.New("end node not found")
	}
	forwardDist := make(map[int64]float64)
	forwardPrev := make(map[int64]int64)
	forwardVisited := make(map[int64]bool)
	backwardDist := make(map[int64]float64)
	backwardPrev := make(map[int64]int64)
	backwardVisited := make(map[int64]bool)
	//Calculate initial heuristic estimates.
	//The estimate is the same for forwards and backwards because haversine is symmetrical.
	hStart := haversine(start.Lat, start.Lon, end.Lat, end.Lon)
	forwardDist[startID] = 0
	backwardDist[endID] = 0
	//Initialize the Priority Queues with their actual f-values (g + h).
	//g = 0, h = hStart
	forwardPQ := &PriorityQueue{{nodeID: startID, distance: hStart}}
	backwardPQ := &PriorityQueue{{nodeID: endID, distance: hStart}}
	heap.Init(forwardPQ)
	heap.Init(backwardPQ)
	bestDist := math.Inf(1)
	fForward := hStart
	fBackward := hStart
	meetingNode := int64(-1)
	forwardVisits := 0
	backwardVisits := 0
	for forwardPQ.Len() > 0 && backwardPQ.Len() > 0 {
		//Early termination: if the minimum possible path cost from either frontier
		//meets or exceeds the best path found so far, we can stop immediately.
		if fForward >= bestDist || fBackward >= bestDist {
			break
		}
		// Forward search step
		forwardCurrent := heap.Pop(forwardPQ).(*Item)
		if !forwardVisited[forwardCurrent.nodeID] {
			forwardNode := g.Nodes[forwardCurrent.nodeID]
			//Rejection criteria for forward search:
			//Prune node if its forward f-value >= bestDist
			//OR if its forward g-value + fBackward - backward_heuristic >= bestDist.
			//Implementation inspired by:
			//Wim Pijls and Henk Post,
			//"Yet Another Bidirectional Algorithm for Shortest Paths" (2009)
			//https://repub.eur.nl/pub/16100/ei2009-10.pdf
			//(Page 2, line 13)
			forwardRejected := forwardDist[forwardCurrent.nodeID]+haversine(forwardNode.Lat, forwardNode.Lon, end.Lat, end.Lon) >= bestDist ||
				forwardDist[forwardCurrent.nodeID]+fBackward-haversine(forwardNode.Lat, forwardNode.Lon, start.Lat, start.Lon) >= bestDist
			if !forwardRejected {
				forwardVisited[forwardCurrent.nodeID] = true
				forwardVisits = forwardVisits + 1
				for _, edge := range g.Edges[forwardCurrent.nodeID] {
					if forwardVisited[edge.To] {
						continue
					}
					neighbor, ok := g.Nodes[edge.To]
					if !ok {
						continue
					}
					newDist := forwardDist[forwardCurrent.nodeID] + edge.Distance
					existing, ok := forwardDist[edge.To]
					if !ok || newDist < existing {
						forwardDist[edge.To] = newDist
						forwardPrev[edge.To] = forwardCurrent.nodeID
						//Update bestDist when meeting the other search frontier (at edge relaxation).
						if bDist, reached := backwardDist[edge.To]; reached {
							if newDist+bDist < bestDist {
								bestDist = newDist + bDist
								meetingNode = edge.To
							}
						}
						//Forward-moving heuristic calculation (g + h).
						h := haversine(neighbor.Lat, neighbor.Lon, end.Lat, end.Lon)
						heap.Push(forwardPQ, &Item{nodeID: neighbor.ID, distance: newDist + h})
					}
				}
			}
		}
		//Update fForward with the new minimum f-value of the forward queue.
		if forwardPQ.Len() > 0 {
			fForward = (*forwardPQ)[0].distance
		} else {
			//Early termination condition.
			fForward = math.Inf(1)
		}
		//Backward search step.
		backwardCurrent := heap.Pop(backwardPQ).(*Item)
		if !backwardVisited[backwardCurrent.nodeID] {
			backwardNode := g.Nodes[backwardCurrent.nodeID]
			//Rejection criteria for backward search, basically the same as the forward search.
			backwardRejected := backwardDist[backwardCurrent.nodeID]+haversine(backwardNode.Lat, backwardNode.Lon, start.Lat, start.Lon) >= bestDist ||
				backwardDist[backwardCurrent.nodeID]+fForward-haversine(backwardNode.Lat, backwardNode.Lon, end.Lat, end.Lon) >= bestDist
			if !backwardRejected {
				backwardVisited[backwardCurrent.nodeID] = true
				backwardVisits = backwardVisits + 1
				for _, edge := range g.Edges[backwardCurrent.nodeID] {
					if backwardVisited[edge.To] {
						continue
					}
					neighbor, ok := g.Nodes[edge.To]
					if !ok {
						continue
					}
					newDist := backwardDist[backwardCurrent.nodeID] + edge.Distance
					existing, ok := backwardDist[edge.To]
					if !ok || newDist < existing {
						backwardDist[edge.To] = newDist
						backwardPrev[edge.To] = backwardCurrent.nodeID
						//Update bestDist when meeting the other search frontier (at edge relaxation).
						if fDist, reached := forwardDist[edge.To]; reached {
							if fDist+newDist < bestDist {
								bestDist = fDist + newDist
								meetingNode = edge.To
							}
						}
						//Backward-moving heuristic calculation (g + h).
						h := haversine(neighbor.Lat, neighbor.Lon, start.Lat, start.Lon)
						heap.Push(backwardPQ, &Item{nodeID: neighbor.ID, distance: newDist + h})
					}
				}
			}
		}
		//Update fBackward with the new minimum f-value of the backward queue.
		if backwardPQ.Len() > 0 {
			fBackward = (*backwardPQ)[0].distance
		} else {
			fBackward = math.Inf(1)
		}
	}
	if meetingNode == -1 {
		return nil, 0, errors.New("no path found")
	}
	//Reconstruct path from both directions.
	var forwardPath []int64
	curr := meetingNode
	for curr != startID {
		forwardPath = append([]int64{curr}, forwardPath...)
		parent, ok := forwardPrev[curr]
		if !ok {
			return nil, 0, fmt.Errorf("Malformed path: node %d has no forward parent.", curr)
		}
		curr = parent
	}
	forwardPath = append([]int64{startID}, forwardPath...)
	var backwardPath []int64
	curr = meetingNode
	for curr != endID {
		parent, ok := backwardPrev[curr]
		if !ok {
			return nil, 0, fmt.Errorf("Malformed path: node %d has no backward parent.", curr)
		}
		curr = parent
		backwardPath = append(backwardPath, curr)
	}
	path := append(forwardPath, backwardPath...)
	fmt.Printf("Bidirectional forward visits: %d, Bidirectional backward visits: %d\n", forwardVisits, backwardVisits)
	return path, bestDist, nil
}
