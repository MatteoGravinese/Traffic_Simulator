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
	forwardTime := make(map[int64]float64)
	forwardPrev := make(map[int64]int64)
	forwardVisited := make(map[int64]bool)
	backwardTime := make(map[int64]float64)
	backwardPrev := make(map[int64]int64)
	backwardVisited := make(map[int64]bool)
	//Calculate initial heuristic estimates.
	//The estimate is the same for forwards and backwards because heuristic is symmetrical.
	hStart := heuristic(start.Lat, start.Lon, end.Lat, end.Lon, g.MaxSpeed)
	forwardTime[startID] = 0
	backwardTime[endID] = 0
	//Initialize the Priority Queues with their actual f-values (g + h).
	//g = 0, h = hStart
	forwardPQ := &PriorityQueue{{nodeID: startID, time: hStart}}
	backwardPQ := &PriorityQueue{{nodeID: endID, time: hStart}}
	heap.Init(forwardPQ)
	heap.Init(backwardPQ)
	bestTime := math.Inf(1)
	fForward := hStart
	fBackward := hStart
	meetingNode := int64(-1)
	for forwardPQ.Len() > 0 && backwardPQ.Len() > 0 {
		//Early termination: if the minimum possible path cost from either frontier
		//meets or exceeds the best path found so far, we can stop immediately.
		if fForward >= bestTime || fBackward >= bestTime {
			break
		}
		// Forward search step
		forwardCurrent := heap.Pop(forwardPQ).(*Item)
		if !forwardVisited[forwardCurrent.nodeID] {
			forwardNode := g.Nodes[forwardCurrent.nodeID]
			//Rejection criteria for forward search:
			//Prune node if its forward f-value >= bestTime
			//OR if its forward g-value + fBackward - backward_heuristic >= bestTime.
			//Implementation inspired by:
			//Wim Pijls and Henk Post,
			//"Yet Another Bidirectional Algorithm for Shortest Paths" (2009)
			//https://repub.eur.nl/pub/16100/ei2009-10.pdf
			//(Page 2, line 13)
			forwardRejected := forwardTime[forwardCurrent.nodeID]+heuristic(forwardNode.Lat, forwardNode.Lon, end.Lat, end.Lon, g.MaxSpeed) >= bestTime ||
				forwardTime[forwardCurrent.nodeID]+fBackward-heuristic(forwardNode.Lat, forwardNode.Lon, start.Lat, start.Lon, g.MaxSpeed) >= bestTime
			if !forwardRejected {
				forwardVisited[forwardCurrent.nodeID] = true
				for _, edge := range g.Edges[forwardCurrent.nodeID] {
					if forwardVisited[edge.To] {
						continue
					}
					neighbor, ok := g.Nodes[edge.To]
					if !ok {
						continue
					}
					newTime := forwardTime[forwardCurrent.nodeID] + edge.Time
					existing, ok := forwardTime[edge.To]
					if !ok || newTime < existing {
						forwardTime[edge.To] = newTime
						forwardPrev[edge.To] = forwardCurrent.nodeID
						//Update bestTime when meeting the other search frontier (at edge relaxation).
						if bTime, reached := backwardTime[edge.To]; reached {
							if newTime+bTime < bestTime {
								bestTime = newTime + bTime
								meetingNode = edge.To
							}
						}
						//Forward-moving heuristic calculation (g + h).
						h := heuristic(neighbor.Lat, neighbor.Lon, end.Lat, end.Lon, g.MaxSpeed)
						heap.Push(forwardPQ, &Item{nodeID: neighbor.ID, time: newTime + h})
					}
				}
			}
		}
		//Update fForward with the new minimum f-value of the forward queue.
		if forwardPQ.Len() > 0 {
			fForward = (*forwardPQ)[0].time
		} else {
			//Early termination condition.
			fForward = math.Inf(1)
		}
		//Backward search step.
		backwardCurrent := heap.Pop(backwardPQ).(*Item)
		if !backwardVisited[backwardCurrent.nodeID] {
			backwardNode := g.Nodes[backwardCurrent.nodeID]
			//Rejection criteria for backward search, basically the same as the forward search.
			backwardRejected := backwardTime[backwardCurrent.nodeID]+heuristic(backwardNode.Lat, backwardNode.Lon, start.Lat, start.Lon, g.MaxSpeed) >= bestTime ||
				backwardTime[backwardCurrent.nodeID]+fForward-heuristic(backwardNode.Lat, backwardNode.Lon, end.Lat, end.Lon, g.MaxSpeed) >= bestTime
			if !backwardRejected {
				backwardVisited[backwardCurrent.nodeID] = true
				for _, edge := range g.Edges[backwardCurrent.nodeID] {
					if backwardVisited[edge.To] {
						continue
					}
					neighbor, ok := g.Nodes[edge.To]
					if !ok {
						continue
					}
					newTime := backwardTime[backwardCurrent.nodeID] + edge.Time
					existing, ok := backwardTime[edge.To]
					if !ok || newTime < existing {
						backwardTime[edge.To] = newTime
						backwardPrev[edge.To] = backwardCurrent.nodeID
						//Update bestTime when meeting the other search frontier (at edge relaxation).
						if fTime, reached := forwardTime[edge.To]; reached {
							if fTime+newTime < bestTime {
								bestTime = fTime + newTime
								meetingNode = edge.To
							}
						}
						//Backward-moving heuristic calculation (g + h).
						h := heuristic(neighbor.Lat, neighbor.Lon, start.Lat, start.Lon, g.MaxSpeed)
						heap.Push(backwardPQ, &Item{nodeID: neighbor.ID, time: newTime + h})
					}
				}
			}
		}
		//Update fBackward with the new minimum f-value of the backward queue.
		if backwardPQ.Len() > 0 {
			fBackward = (*backwardPQ)[0].time
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
	return path, bestTime, nil
}
