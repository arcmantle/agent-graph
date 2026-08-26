package benchmark_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"agent-graph/benchmark"
	"agent-graph/graph"
	"agent-graph/index"
	"agent-graph/storage"
	"agent-graph/storage/sqlite"
)

func TestValidateAcceptsOneInLimitRun(t *testing.T) {
	runs := []benchmark.Run{
		{Measurements: measurements(60*time.Second, 2*time.Second, 500*time.Millisecond, 500*time.Millisecond, 500*time.Millisecond)},
	}

	if err := benchmark.Validate(runs); err != nil {
		t.Fatalf("validate in-limit medians: %v", err)
	}
}

func TestValidateRejectsMedianAboveApprovedLimit(t *testing.T) {
	runs := []benchmark.Run{
		{Measurements: measurements(61*time.Second, 2*time.Second, 200*time.Millisecond, 200*time.Millisecond, 200*time.Millisecond)},
	}

	err := benchmark.Validate(runs)
	if err == nil {
		t.Fatal("validate over-limit median succeeded")
	}
	if !strings.Contains(err.Error(), "initial_index") || !strings.Contains(err.Error(), "1m0s") {
		t.Errorf("validate error = %q, want operation and approved limit", err)
	}
}

func TestValidateReportRejectsMissingRunMetadata(t *testing.T) {
	runs := []benchmark.Run{
		{Measurements: measurements(time.Second, time.Second, time.Millisecond, time.Millisecond, time.Millisecond), PhaseMeasurements: phaseMeasurements(), ResolverMeasurements: resolverMeasurements(), SQLiteWriteMeasurements: sqliteWriteMeasurements()},
	}

	err := benchmark.ValidateReport(runs)
	if err == nil {
		t.Fatal("validate report with missing metadata succeeded")
	}
	if !strings.Contains(err.Error(), "peak RSS") {
		t.Errorf("validate report error = %q, want missing metadata", err)
	}
}

func TestValidateReportRejectsDifferentChecksums(t *testing.T) {
	runs := []benchmark.Run{
		{Measurements: measurements(time.Second, time.Second, time.Millisecond, time.Millisecond, time.Millisecond)},
	}

	err := benchmark.ValidateReport(runs)
	if err == nil {
		t.Fatal("validate report with incomplete run metadata succeeded")
	}
	if !strings.Contains(err.Error(), "phase measurements") {
		t.Errorf("validate report error = %q, want missing phase measurements", err)
	}
}

func TestValidateReportRejectsIncompletePhaseMeasurements(t *testing.T) {
	runs := []benchmark.Run{{
		Measurements:            measurements(time.Second, time.Second, time.Millisecond, time.Millisecond, time.Millisecond),
		ResolverMeasurements:    resolverMeasurements(),
		SQLiteWriteMeasurements: sqliteWriteMeasurements(),
		PeakRSSBytes:            1,
		DatabaseBytes:           1,
		OutputChecksum:          "sha256:stable",
	}}

	err := benchmark.ValidateReport(runs)
	if err == nil {
		t.Fatal("validate report without indexing phases succeeded")
	}
	if !strings.Contains(err.Error(), "phase measurements") {
		t.Errorf("validate report error = %q, want missing phase measurements", err)
	}
}

func TestValidateReportRejectsInvalidResolverMeasurements(t *testing.T) {
	testCases := []struct {
		name         string
		measurements []benchmark.Measurement
	}{
		{name: "missing", measurements: nil},
		{name: "wrong order", measurements: []benchmark.Measurement{
			{Name: "contribution_restoration", Duration: time.Millisecond},
			{Name: "affected_source_selection", Duration: time.Millisecond},
			{Name: "workspace_resolution", Duration: time.Millisecond},
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			runs := []benchmark.Run{{
				Measurements:            measurements(time.Second, time.Second, time.Millisecond, time.Millisecond, time.Millisecond),
				PhaseMeasurements:       phaseMeasurements(),
				ResolverMeasurements:    testCase.measurements,
				SQLiteWriteMeasurements: sqliteWriteMeasurements(),
				PeakRSSBytes:            1,
				DatabaseBytes:           1,
				OutputChecksum:          "sha256:stable",
			}}

			err := benchmark.ValidateReport(runs)
			if err == nil {
				t.Fatal("validate report with invalid resolver measurements succeeded")
			}
			if !strings.Contains(err.Error(), "resolver measurements") {
				t.Errorf("validate report error = %q, want resolver measurement error", err)
			}
		})
	}
}

func TestGenerateCorpusCreatesDeterministicSourceFanout(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	specification := benchmark.CorpusSpec{SourceFiles: 3, FunctionsPerFile: 4}

	first, err := benchmark.GenerateCorpus(firstRoot, specification)
	if err != nil {
		t.Fatalf("generate first corpus: %v", err)
	}
	second, err := benchmark.GenerateCorpus(secondRoot, specification)
	if err != nil {
		t.Fatalf("generate second corpus: %v", err)
	}

	if first.SourceFiles != 3 || first.FunctionsPerFile != 4 || first.UpdatePath == "" || first.QueryTerm == "" || first.PathSource == "" || first.PathTarget == "" || first.ExplainTerm == "" {
		t.Errorf("first corpus = %+v, want source details and benchmark targets", first)
	}
	if first != second {
		t.Errorf("corpus metadata differs: first = %+v, second = %+v", first, second)
	}
	for sourceIndex := 0; sourceIndex < specification.SourceFiles; sourceIndex++ {
		path := filepath.Join("src", "module-"+strconv.Itoa(sourceIndex)+".ts")
		firstContents, err := os.ReadFile(filepath.Join(firstRoot, path))
		if err != nil {
			t.Fatalf("read first source %q: %v", path, err)
		}
		secondContents, err := os.ReadFile(filepath.Join(secondRoot, path))
		if err != nil {
			t.Fatalf("read second source %q: %v", path, err)
		}
		if string(firstContents) != string(secondContents) {
			t.Errorf("source %q differs across generated corpora", path)
		}
	}

	contents, err := os.ReadFile(filepath.Join(firstRoot, first.UpdatePath))
	if err != nil {
		t.Fatalf("read update source: %v", err)
	}
	if !strings.Contains(string(contents), "import") || !strings.Contains(string(contents), "function") {
		t.Errorf("update source = %q, want imports and functions", contents)
	}
}

func TestGenerateCorpusDistributesAdditionalFunctionsAcrossFiles(t *testing.T) {
	root := t.TempDir()
	specification := benchmark.CorpusSpec{
		SourceFiles:         4,
		FunctionsPerFile:    2,
		AdditionalFunctions: 3,
		UncalledFunctions:   2,
	}
	if _, err := benchmark.GenerateCorpus(root, specification); err != nil {
		t.Fatalf("generate corpus: %v", err)
	}

	for sourceIndex, wantFunctions := range []int{3, 3, 3, 2} {
		contents, err := os.ReadFile(filepath.Join(root, "src", "module-"+strconv.Itoa(sourceIndex)+".ts"))
		if err != nil {
			t.Fatalf("read source %d: %v", sourceIndex, err)
		}
		if got := strings.Count(string(contents), "export function"); got != wantFunctions {
			t.Errorf("source %d function count = %d, want %d", sourceIndex, got, wantFunctions)
		}
	}
}

func TestExactScaleCorpusSpecProducesExactNodeAndEdgeTargets(t *testing.T) {
	for _, sourceFiles := range []int{1000, 10000} {
		specification, err := benchmark.ExactScaleCorpusSpec(sourceFiles)
		if err != nil {
			t.Fatalf("create exact scale specification for %d files: %v", sourceFiles, err)
		}
		root := t.TempDir()
		corpus, err := benchmark.GenerateCorpus(root, specification)
		if err != nil {
			t.Fatalf("generate exact scale corpus for %d files: %v", sourceFiles, err)
		}
		if corpus.ExpectedNodes != sourceFiles*100 || corpus.ExpectedEdges != sourceFiles*200 {
			t.Errorf("%d-file corpus counts = %d nodes and %d edges, want %d and %d", sourceFiles, corpus.ExpectedNodes, corpus.ExpectedEdges, sourceFiles*100, sourceFiles*200)
		}
	}
}

func TestGeneratedCorpusProducesExpectedGraphEvidence(t *testing.T) {
	root := t.TempDir()
	corpus, err := benchmark.GenerateCorpus(root, benchmark.CorpusSpec{SourceFiles: 3, FunctionsPerFile: 4})
	if err != nil {
		t.Fatalf("generate corpus: %v", err)
	}
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "benchmark.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer store.Close()

	result, err := index.Index(context.Background(), store, index.Request{Root: root})
	if err != nil {
		t.Fatalf("index corpus: %v", err)
	}
	facts := graph.Facts{}
	if err := store.Export(context.Background(), result.Snapshot, storage.ExportRequest{}, factCollector{facts: &facts}); err != nil {
		t.Fatalf("export corpus facts: %v", err)
	}

	if len(facts.Nodes) != 16 {
		t.Errorf("node count = %d, want 16", len(facts.Nodes))
	}
	if len(facts.Edges) != 30 {
		t.Errorf("edge count = %d, want 30", len(facts.Edges))
	}
	if !containsNode(facts.Nodes, corpus.PathSource) || !containsNode(facts.Nodes, corpus.PathTarget) || !containsNode(facts.Nodes, corpus.ExplainTerm) {
		t.Errorf("graph nodes do not contain benchmark targets: %+v", facts.Nodes)
	}
}

type factCollector struct {
	facts *graph.Facts
}

func (collector factCollector) WriteNode(node graph.Node) error {
	collector.facts.Nodes = append(collector.facts.Nodes, node)
	return nil
}

func (collector factCollector) WriteEdge(edge graph.Edge) error {
	collector.facts.Edges = append(collector.facts.Edges, edge)
	return nil
}

func containsNode(nodes []graph.Node, qualifiedName string) bool {
	for _, node := range nodes {
		if node.QualifiedName == qualifiedName || node.Label == qualifiedName {
			return true
		}
	}
	return false
}

func measurements(initialIndex, incrementalUpdate, query, path, explain time.Duration) []benchmark.Measurement {
	return []benchmark.Measurement{
		{Name: "initial_index", Duration: initialIndex},
		{Name: "incremental_update", Duration: incrementalUpdate},
		{Name: "query", Duration: query},
		{Name: "path", Duration: path},
		{Name: "explain", Duration: explain},
	}
}

func phaseMeasurements() []benchmark.Measurement {
	return []benchmark.Measurement{
		{Name: "extraction", Duration: time.Millisecond},
		{Name: "resolution", Duration: time.Millisecond},
		{Name: "publication_preparation", Duration: time.Millisecond},
		{Name: "sqlite_write", Duration: time.Millisecond},
		{Name: "commit", Duration: time.Millisecond},
	}
}

func resolverMeasurements() []benchmark.Measurement {
	return []benchmark.Measurement{
		{Name: "affected_source_selection", Duration: time.Millisecond},
		{Name: "contribution_restoration", Duration: time.Millisecond},
		{Name: "workspace_resolution", Duration: time.Millisecond},
	}
}

func sqliteWriteMeasurements() []benchmark.Measurement {
	return []benchmark.Measurement{
		{Name: "workspace_nodes", Duration: time.Millisecond},
		{Name: "workspace_edges", Duration: time.Millisecond},
		{Name: "file_contributions", Duration: time.Millisecond},
		{Name: "contribution_nodes", Duration: time.Millisecond},
		{Name: "contribution_edges", Duration: time.Millisecond},
		{Name: "contribution_extensions", Duration: time.Millisecond},
		{Name: "contribution_dependencies", Duration: time.Millisecond},
		{Name: "contribution_exported_surfaces", Duration: time.Millisecond},
		{Name: "contribution_diagnostics", Duration: time.Millisecond},
		{Name: "contribution_unresolved_references", Duration: time.Millisecond},
		{Name: "contribution_module_bindings", Duration: time.Millisecond},
		{Name: "contribution_symbol_references", Duration: time.Millisecond},
	}
}
