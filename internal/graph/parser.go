package graph

import (
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"strings"
)

// OSM structs mirror the XML structure of the .osm file.
type OSMNode struct {
	ID  int64   `xml:"id,attr"`
	Lat float64 `xml:"lat,attr"`
	Lon float64 `xml:"lon,attr"`
}

type OSMWay struct {
	Nodes []OSMNodeRef `xml:"nd"`
	Tags  []OSMTag     `xml:"tag"`
}

type OSMNodeRef struct {
	Ref int64 `xml:"ref,attr"`
}

type OSMTag struct {
	Key   string `xml:"k,attr"`
	Value string `xml:"v,attr"`
}

type OSMData struct {
	Nodes []OSMNode `xml:"node"`
	Ways  []OSMWay  `xml:"way"`
}

// ParseOSM reads an .osm file and builds a graph from it.
func ParseOSM(filename string) (*Graph, error) {
	routableHighways := map[string]bool{
		"motorway": true, "motorway_link": true,
		"trunk": true, "trunk_link": true,
		"primary": true, "primary_link": true,
		"secondary": true, "secondary_link": true,
		"tertiary": true, "tertiary_link": true,
		"residential": true, "living_street": true,
		"unclassified": true, "service": true,
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var osmData OSMData
	decoder := xml.NewDecoder(file)
	if err := decoder.Decode(&osmData); err != nil {
		return nil, err
	}
	g := NewGraph()
	//First pass: collect node IDs referenced by routable ways.
	referencedNodes := make(map[int64]bool)
	for _, way := range osmData.Ways {
		highway := ""
		for _, tag := range way.Tags {
			if tag.Key == "highway" {
				highway = tag.Value
			}
		}
		if !routableHighways[highway] {
			continue
		}
		for _, n := range way.Nodes {
			referencedNodes[n.Ref] = true
		}
	}
	//Add only routable nodes to the graph.
	for _, n := range osmData.Nodes {
		if !referencedNodes[n.ID] {
			continue
		}
		g.AddNode(Node{
			ID:  n.ID,
			Lat: n.Lat,
			Lon: n.Lon,
		})
	}
	//Add all edges from ways.
	for _, way := range osmData.Ways {
		lanes := 2
		highway := ""
		speed := 0.0
		speedSet := false
		oneway := false
		onewayReverse := false
		for _, tag := range way.Tags {
			if tag.Key == "highway" {
				highway = tag.Value
			}
			if tag.Key == "maxspeed" {
				if strings.Contains(tag.Value, "mph") {
					fmt.Sscanf(tag.Value, "%f mph", &speed)
				} else {
					//Assume km/h, convert to mph.
					var kmh float64
					fmt.Sscanf(tag.Value, "%f", &kmh)
					speed = kmh * 0.621371
				}
				speedSet = true
			}
			if tag.Key == "lanes" {
				fmt.Sscanf(tag.Value, "%d", &lanes)
			}
			if tag.Key == "oneway" {
				switch tag.Value {
				case "yes", "1", "true":
					oneway = true
				case "-1":
					onewayReverse = true
				}
			}
		}
		if highway == "motorway" {
			oneway = true
		}
		if !speedSet {
			speed = highwayDefaultSpeed(highway)
		}
		for i := 0; i < len(way.Nodes)-1; i++ {
			fromID := way.Nodes[i].Ref
			toID := way.Nodes[i+1].Ref
			from, ok1 := g.Nodes[fromID]
			to, ok2 := g.Nodes[toID]
			if !ok1 || !ok2 {
				continue
			}
			dist := haversine(from.Lat, from.Lon, to.Lat, to.Lon)
			time := dist / speed
			if !onewayReverse {
				g.AddEdge(fromID, Edge{To: toID, Distance: dist, Speed: speed, Lanes: lanes, Time: time, Highway: highway})
			}
			if !oneway {
				g.AddEdge(toID, Edge{To: fromID, Distance: dist, Speed: speed, Lanes: lanes, Time: time, Highway: highway})
			}
		}
	}
	var maxSpeed float64
	for _, edges := range g.Edges {
		for _, edge := range edges {
			if edge.Speed > maxSpeed {
				maxSpeed = edge.Speed
			}
		}
	}
	g.MaxSpeed = maxSpeed
	return g, nil
}

// Default speed based off of road type.
func highwayDefaultSpeed(highway string) float64 {
	switch highway {
	case "motorway":
		return 65.0
	case "motorway_link":
		return 45.0
	case "trunk":
		return 55.0
	case "trunk_link":
		return 45.0
	case "primary":
		return 45.0
	case "primary_link":
		return 35.0
	case "secondary":
		return 35.0
	case "secondary_link":
		return 25.0
	case "tertiary":
		return 25.0
	case "tertiary_link":
		return 25.0
	case "residential":
		return 25.0
	case "living_street":
		return 15.0
	case "unclassified":
		return 25.0
	case "service":
		return 15.0
	default:
		return 25.0
	}
}

// haversine calculates the distance in miles between two lat/lon points.
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 3958.8
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func heuristic(lat1, lon1, lat2, lon2, maxSpeed float64) float64 {
	return haversine(lat1, lon1, lat2, lon2) / maxSpeed
}
