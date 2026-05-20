package graph

//Nodes represent intersections on the map.
type Node struct {
	ID  int64
	Lat float64
	Lon float64
}

//Edges represent roads connecting two nodes.
type Edge struct {
	To       int64
	Distance float64
	Speed    float64
}

//Graph holds all nodes and edges in memory.
type Graph struct {
	Nodes map[int64]Node
	Edges map[int64][]Edge
}

//NewGraph creates an empty graph.
func NewGraph() *Graph {
	return &Graph{
		Nodes: make(map[int64]Node),
		Edges: make(map[int64][]Edge),
	}
}

//AddNode adds an intersection to the graph.
func (g *Graph) AddNode(n Node) {
	g.Nodes[n.ID] = n
}

//AddEdge adds a road between two nodes.
func (g *Graph) AddEdge(from int64, e Edge) {
	g.Edges[from] = append(g.Edges[from], e)
}
