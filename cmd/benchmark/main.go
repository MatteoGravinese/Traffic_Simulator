package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/MatteoGravinese/Traffic_Simulator/internal/graph"
	"github.com/MatteoGravinese/Traffic_Simulator/internal/osmpath"
)

const (
	readmePath      = "README.md"
	numQueries      = 100
	reroutesPerTick = 30 // matches maxVehiclesPerTick in simulation.Tick
	numTicks        = 20
)

const benchmarkStart = "<!-- BENCHMARK:START -->"
const benchmarkEnd = "<!-- BENCHMARK:END -->"

const (
	pittsburghSearch        = "Pittsburgh, Pennsylvania"
	benchmarkUserAgent      = "TrafficSimulator-Benchmark/1.0"
	pittsburghHighwayFilter = `footway|cycleway|path|steps|pedestrian|track`
)

type nominatimResult struct {
	OSMType string `json:"osm_type"`
	OSMID   int64  `json:"osm_id"`
}

type benchResult struct {
	nodes             int
	edges             int
	chPreprocess      time.Duration
	cchPreprocess     time.Duration
	queriesRun        int
	dijkstraAvg       time.Duration
	chAvg             time.Duration
	cchAvg            time.Duration
	customizeAvg      time.Duration
	dijkstraTickAvg   time.Duration
	cchTickAvg        time.Duration
	tickSpeedupVsDijk float64
}

func main() {
	if err := ensureOSM(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to get OSM file: %v\n", err)
		os.Exit(1)
	}

	result, err := runBenchmark()
	if err != nil {
		fmt.Fprintf(os.Stderr, "benchmark failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(benchmarkBody(result))

	if err := patchREADME(formatREADMEReport(result)); err != nil {
		fmt.Fprintf(os.Stderr, "failed to update README: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("README.md updated.")
}

func ensureOSM() error {
	if _, err := os.Stat(osmpath.PittsburghOSM); err == nil {
		return nil
	}
	if err := os.MkdirAll(osmpath.Dir, 0755); err != nil {
		return err
	}

	fmt.Printf("Downloading Pittsburgh road network to %s ...\n", osmpath.PittsburghOSM)

	client := &http.Client{Timeout: 11 * time.Minute}
	areaID, err := resolveOverpassAreaID(client, pittsburghSearch)
	if err != nil {
		return fmt.Errorf("resolve Pittsburgh area: %w", err)
	}

	query := pittsburghOverpassQuery(areaID)
	mirrors := []string{
		"https://overpass-api.de/api/interpreter",
		"https://overpass.kumi.systems/api/interpreter",
		"https://overpass.openstreetmap.fr/api/interpreter",
	}

	var lastErr error
	for _, mirror := range mirrors {
		data := url.Values{}
		data.Set("data", query)
		req, err := http.NewRequest(http.MethodPost, mirror, strings.NewReader(data.Encode()))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", benchmarkUserAgent)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			lastErr = fmt.Errorf("%s returned HTTP %s: %s", mirror, resp.Status, strings.TrimSpace(string(body)))
			continue
		}

		out, err := os.Create(osmpath.PittsburghOSM)
		if err != nil {
			resp.Body.Close()
			return err
		}
		_, copyErr := io.Copy(out, resp.Body)
		out.Close()
		resp.Body.Close()
		if copyErr != nil {
			os.Remove(osmpath.PittsburghOSM)
			lastErr = copyErr
			continue
		}
		fmt.Println("Download complete.")
		return nil
	}
	return fmt.Errorf("all Overpass mirrors failed: %w", lastErr)
}

// resolveOverpassAreaID maps a place name to an Overpass area ID via Nominatim.
// {{geocodeArea:...}} in Overpass Turbo is not valid Overpass QL on the API.
func resolveOverpassAreaID(client *http.Client, search string) (int64, error) {
	endpoint := "https://nominatim.openstreetmap.org/search?" + url.Values{
		"q":      {search},
		"format": {"json"},
		"limit":  {"1"},
	}.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", benchmarkUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return 0, fmt.Errorf("nominatim HTTP %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var results []nominatimResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return 0, err
	}
	if len(results) == 0 {
		return 0, fmt.Errorf("no results for %q", search)
	}

	switch results[0].OSMType {
	case "relation":
		return 3600000000 + results[0].OSMID, nil
	case "way":
		return 2400000000 + results[0].OSMID, nil
	default:
		return 0, fmt.Errorf("unsupported osm_type %q for %q", results[0].OSMType, search)
	}
}

func pittsburghOverpassQuery(areaID int64) string {
	return fmt.Sprintf(`[out:xml][timeout:600];
area(%d)->.searchArea;
(
  way(area.searchArea)
    ["highway"]
    ["highway"!~"%s"];
);
(._;>;);
out body;`, areaID, pittsburghHighwayFilter)
}

func runBenchmark() (*benchResult, error) {
	fmt.Printf("Parsing %s ...\n", osmpath.PittsburghOSM)
	g, err := graph.ParseOSM(osmpath.PittsburghOSM, nil)
	if err != nil {
		return nil, err
	}

	nodeIDs := make([]int64, 0, len(g.Nodes))
	for id := range g.Nodes {
		nodeIDs = append(nodeIDs, id)
	}
	if len(nodeIDs) < 2 {
		return nil, fmt.Errorf("graph has fewer than 2 nodes")
	}

	result := &benchResult{
		nodes: len(g.Nodes),
		edges: g.EdgeCount(),
	}

	fmt.Println("Preprocessing CH ...")
	chStart := time.Now()
	ch := graph.Preprocess(g, nil)
	result.chPreprocess = time.Since(chStart)

	fmt.Println("Preprocessing CCH ...")
	cchStart := time.Now()
	cch := graph.PreprocessCCH(g, nil)
	result.cchPreprocess = time.Since(cchStart)

	pairs := samplePairs(nodeIDs, numQueries*3, ch)
	if len(pairs) < reroutesPerTick {
		return nil, fmt.Errorf("need at least %d routable pairs, got %d", reroutesPerTick, len(pairs))
	}

	warmup := pairs[0]
	graph.Dijkstra(g, warmup[0], warmup[1])
	graph.CHQuery(ch, warmup[0], warmup[1])
	cch.Customize(randomCongestion(g, rand.New(rand.NewSource(0)), 0.05))
	graph.CCHQuery(cch, warmup[0], warmup[1])

	fmt.Println("Benchmarking single-query latency ...")
	var dijkstraTotal, chTotal, cchTotal time.Duration
	queriesRun := 0
	for _, pair := range pairs {
		if queriesRun >= numQueries {
			break
		}
		startID, endID := pair[0], pair[1]

		t0 := time.Now()
		_, _, err := graph.Dijkstra(g, startID, endID)
		if err != nil {
			continue
		}
		dijkstraTotal += time.Since(t0)

		t0 = time.Now()
		_, _, err = graph.CHQuery(ch, startID, endID)
		if err != nil {
			continue
		}
		chTotal += time.Since(t0)

		t0 = time.Now()
		_, _, err = graph.CCHQuery(cch, startID, endID)
		if err != nil {
			continue
		}
		cchTotal += time.Since(t0)

		queriesRun++
	}
	if queriesRun == 0 {
		return nil, fmt.Errorf("all single queries failed")
	}

	result.queriesRun = queriesRun
	result.dijkstraAvg = dijkstraTotal / time.Duration(queriesRun)
	result.chAvg = chTotal / time.Duration(queriesRun)
	result.cchAvg = cchTotal / time.Duration(queriesRun)

	fmt.Printf("Benchmarking traffic-aware tick (%d reroutes/tick, %d ticks) ...\n", reroutesPerTick, numTicks)
	tickPairs := pairs[:reroutesPerTick]
	tickRNG := rand.New(rand.NewSource(99))

	var customizeTotal, dijkstraTickTotal, cchTickTotal time.Duration
	for tick := 0; tick < numTicks; tick++ {
		congestion := randomCongestion(g, tickRNG, 0.05)

		t0 := time.Now()
		cch.Customize(congestion)
		customizeTotal += time.Since(t0)

		for _, pair := range tickPairs {
			startID, endID := pair[0], pair[1]

			t0 = time.Now()
			_, _, err := graph.Dijkstra(g, startID, endID)
			if err != nil {
				return nil, fmt.Errorf("dijkstra tick query failed: %w", err)
			}
			dijkstraTickTotal += time.Since(t0)

			t0 = time.Now()
			_, _, err = graph.CCHQuery(cch, startID, endID)
			if err != nil {
				return nil, fmt.Errorf("cch tick query failed: %w", err)
			}
			cchTickTotal += time.Since(t0)
		}
	}

	result.customizeAvg = customizeTotal / numTicks
	result.dijkstraTickAvg = dijkstraTickTotal / numTicks
	result.cchTickAvg = (customizeTotal / numTicks) + (cchTickTotal / numTicks)
	if result.cchTickAvg > 0 {
		result.tickSpeedupVsDijk = float64(result.dijkstraTickAvg) / float64(result.cchTickAvg)
	}

	return result, nil
}

func samplePairs(nodeIDs []int64, maxAttempts int, ch *graph.CHGraph) [][2]int64 {
	rng := rand.New(rand.NewSource(42))
	pairs := make([][2]int64, 0, numQueries)
	for attempt := 0; attempt < maxAttempts && len(pairs) < numQueries; attempt++ {
		startID := nodeIDs[rng.Intn(len(nodeIDs))]
		endID := nodeIDs[rng.Intn(len(nodeIDs))]
		if startID == endID {
			continue
		}
		if _, _, err := graph.CHQuery(ch, startID, endID); err != nil {
			continue
		}
		pairs = append(pairs, [2]int64{startID, endID})
	}
	return pairs
}

func randomCongestion(g *graph.Graph, rng *rand.Rand, edgeFraction float64) map[int64]map[int64]float64 {
	congestion := make(map[int64]map[int64]float64)
	for fromID, edges := range g.Edges {
		for _, e := range edges {
			if rng.Float64() >= edgeFraction {
				continue
			}
			if congestion[fromID] == nil {
				congestion[fromID] = make(map[int64]float64)
			}
			congestion[fromID][e.To] = rng.Float64()
		}
	}
	return congestion
}

func formatREADMEReport(r *benchResult) string {
	return "## Pathfinding benchmark (Pittsburgh.osm)\n\n" +
		"Dataset: `testdata/Pittsburgh.osm` (Pittsburgh drivable highways via Nominatim + Overpass area query; downloaded on first run, gitignored). " +
		"Re-run with `go run ./cmd/benchmark`.\n\n" +
		benchmarkBody(r)
}

func benchmarkBody(r *benchResult) string {
	chSpeedup := float64(r.dijkstraAvg) / float64(r.chAvg)
	cchQuerySpeedup := float64(r.dijkstraAvg) / float64(r.cchAvg)
	now := time.Now().UTC().Format("2006-01-02")

	metricsTable := formatCenteredMarkdownTable(
		[]string{"Metric", "Value"},
		[][]string{
			{"Last run (UTC)", now},
			{"Nodes", formatInt(r.nodes)},
			{"Directed edges", formatInt(r.edges)},
			{"CH preprocess", r.chPreprocess.Round(time.Millisecond).String()},
			{"CCH preprocess", r.cchPreprocess.Round(time.Millisecond).String()},
		},
	)

	queryTable := formatCenteredMarkdownTable(
		[]string{"Algorithm", "Avg query time", "Speedup vs Dijkstra"},
		[][]string{
			{"Dijkstra", r.dijkstraAvg.Round(time.Microsecond).String(), "1.0×"},
			{"CH", r.chAvg.Round(time.Microsecond).String(), fmt.Sprintf("%.1f×", chSpeedup)},
			{"CCH query only", r.cchAvg.Round(time.Microsecond).String(), fmt.Sprintf("%.1f×", cchQuerySpeedup)},
		},
	)

	cchTickDetail := fmt.Sprintf("%s (customize %s + queries %s)",
		r.cchTickAvg.Round(time.Millisecond),
		r.customizeAvg.Round(time.Millisecond),
		(r.cchTickAvg - r.customizeAvg).Round(time.Millisecond),
	)
	tickTable := formatCenteredMarkdownTable(
		[]string{"Approach", "Avg tick time", "Speedup vs Dijkstra tick"},
		[][]string{
			{fmt.Sprintf("Dijkstra × %d", reroutesPerTick), r.dijkstraTickAvg.Round(time.Millisecond).String(), "1.0×"},
			{fmt.Sprintf("CH × %d", reroutesPerTick), "**DOES NOT APPLY**", "**DOES NOT APPLY**"},
			{fmt.Sprintf("Customize + CCH × %d", reroutesPerTick), cchTickDetail, fmt.Sprintf("%.1f×", r.tickSpeedupVsDijk)},
		},
	)

	return fmt.Sprintf("%s\n\n"+
		"### Single-query latency (static weights)\n\n"+
		"Averaged over %d random origin–destination pairs (seed=42). Useful for one-off routing on a fixed graph.\n\n"+
		"%s\n\n"+
		"CCH query-only timing uses one free-flow customization before the loop; isolated CCH queries are not the intended production path.\n\n"+
		"### Traffic-aware tick (%d reroutes per tick)\n\n"+
		"Models one simulation tick with traffic-aware rerouting: **%d Dijkstra queries** vs **one Customize (random 5%% edge congestion) + %d CCH queries**, averaged over %d ticks. Matches `maxVehiclesPerTick` in the simulator.\n\n"+
		"%s\n\n"+
		"CH does not apply to traffic-aware ticks: it is preprocessed for static edge weights and cannot be re-customized each tick when congestion changes.\n\n"+
		"**Takeaway:** CH is fastest when weights are static. CCH is for traffic-aware ticks — one Customize per tick plus batch reroutes beats running Dijkstra on every vehicle while still reacting to congestion.\n",
		metricsTable,
		r.queriesRun,
		queryTable,
		reroutesPerTick,
		reroutesPerTick,
		reroutesPerTick,
		numTicks,
		tickTable,
	)
}

const minTableColWidth = 5 // GFM needs at least three dashes between colons (":---:").

func cellByteWidth(text string) int {
	return len(text)
}

func centerTableCell(text string, width int) string {
	if len(text) >= width {
		return text
	}
	pad := width - len(text)
	left := pad / 2
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", pad-left)
}

func separatorTableCell(width int) string {
	if width < minTableColWidth {
		width = minTableColWidth
	}
	return ":" + strings.Repeat("-", width-2) + ":"
}

func writeTableRow(b *strings.Builder, cells []string, widths []int, separator bool) {
	b.WriteString("|")
	for i, width := range widths {
		b.WriteString(" ")
		if separator {
			b.WriteString(separatorTableCell(width))
		} else {
			b.WriteString(centerTableCell(cells[i], width))
		}
		b.WriteString(" |")
	}
}

func formatCenteredMarkdownTable(headers []string, rows [][]string) string {
	widths := make([]int, len(headers))
	for i := range widths {
		widths[i] = minTableColWidth
	}
	for i, header := range headers {
		widths[i] = max(widths[i], cellByteWidth(header))
	}
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], cellByteWidth(cell))
		}
	}

	var b strings.Builder
	writeTableRow(&b, headers, widths, false)
	b.WriteString("\n")
	writeTableRow(&b, nil, widths, true)
	for _, row := range rows {
		b.WriteString("\n")
		writeTableRow(&b, row, widths, false)
	}
	return b.String()
}

func formatInt(n int) string {
	s := fmt.Sprintf("%d", n)
	if n < 1000 {
		return s
	}
	var out []string
	for len(s) > 3 {
		out = append([]string{s[len(s)-3:]}, out...)
		s = s[:len(s)-3]
	}
	if s != "" {
		out = append([]string{s}, out...)
	}
	return strings.Join(out, ",")
}

func patchREADME(report string) error {
	content, err := os.ReadFile(readmePath)
	if err != nil {
		return err
	}
	text := string(content)
	block := benchmarkStart + "\n" + report + benchmarkEnd

	start := strings.Index(text, benchmarkStart)
	end := strings.Index(text, benchmarkEnd)
	var updated string
	if start >= 0 && end > start {
		updated = text[:start] + block + text[end+len(benchmarkEnd):]
	} else {
		updated = strings.TrimRight(text, "\n") + "\n\n" + block + "\n"
	}
	return os.WriteFile(readmePath, []byte(updated), 0644)
}
