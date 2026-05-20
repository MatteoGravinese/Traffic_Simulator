package graph

import (
	"encoding/xml"
	"math"
	"os"
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
	//Add all nodes to the graph.
	for _, n := range osmData.Nodes {
		g.AddNode(Node{
			ID:  n.ID,
			Lat: n.Lat,
			Lon: n.Lon,
		})
	}
	//Add all edges from ways.
	for _, way := range osmData.Ways {
		for i := 0; i < len(way.Nodes)-1; i++ {
			fromID := way.Nodes[i].Ref
			toID := way.Nodes[i+1].Ref
			from, ok1 := g.Nodes[fromID]
			to, ok2 := g.Nodes[toID]
			if !ok1 || !ok2 {
				continue
			}
			dist := haversine(from.Lat, from.Lon, to.Lat, to.Lon)
			g.AddEdge(fromID, Edge{To: toID, Distance: dist, Speed: 30})
			g.AddEdge(toID, Edge{To: fromID, Distance: dist, Speed: 30})
		}
	}
	return g, nil
}

// haversine calculates the distance in km between two lat/lon points.
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 3958.8
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
