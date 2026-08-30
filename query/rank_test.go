package query_test

import (
	"context"
	"reflect"
	"testing"

	"agent-atlas/graph"
	"agent-atlas/query"
	"agent-atlas/storage"
)

func TestRankPrioritizesMatchKindsAndPreservesTerms(t *testing.T) {
	nodes := []graph.Node{
		{ID: "function:target", Label: "other", QualifiedName: "src/other.ts::other", Evidence: graph.FactEvidence{Span: graph.SourceSpan{Path: "src/other.ts"}}},
		{ID: "function:qualified", Label: "other", QualifiedName: "target", Evidence: graph.FactEvidence{Span: graph.SourceSpan{Path: "src/qualified.ts"}}},
		{ID: "function:label", Label: "target", QualifiedName: "src/label.ts::target", Evidence: graph.FactEvidence{Span: graph.SourceSpan{Path: "src/label.ts"}}},
		{ID: "function:prefix", Label: "targetValue", QualifiedName: "src/prefix.ts::targetValue", Evidence: graph.FactEvidence{Span: graph.SourceSpan{Path: "src/prefix.ts"}}},
		{ID: "function:token", Label: "other", QualifiedName: "src/token.ts::run-target-task", Evidence: graph.FactEvidence{Span: graph.SourceSpan{Path: "src/token.ts"}}},
		{ID: "function:path", Label: "other", QualifiedName: "src/path.ts::other", Evidence: graph.FactEvidence{Span: graph.SourceSpan{Path: "src/target-file.ts"}}},
		{ID: "function:substring", Label: "otherTarget", QualifiedName: "src/substring.ts::otherTarget", Evidence: graph.FactEvidence{Span: graph.SourceSpan{Path: "src/substring.ts"}}},
	}

	seeds := query.Rank(nodes, []string{"target", "function:target", "target-file"})
	if len(seeds) != 3 {
		t.Fatalf("seed sets = %d, want 3", len(seeds))
	}
	if seeds[0].Term != "target" {
		t.Errorf("first term = %q, want target", seeds[0].Term)
	}
	if got, want := nodeIDs(seeds[0].Nodes), []string{"function:qualified", "function:label", "function:prefix"}; !reflect.DeepEqual(got, want) {
		t.Errorf("target seed IDs = %v, want %v", got, want)
	}
	if seeds[1].Term != "function:target" {
		t.Errorf("second term = %q, want function:target", seeds[1].Term)
	}
	if got, want := nodeIDs(seeds[1].Nodes), []string{"function:target"}; !reflect.DeepEqual(got, want) {
		t.Errorf("exact ID seed IDs = %v, want %v", got, want)
	}
	if seeds[2].Term != "target-file" {
		t.Errorf("third term = %q, want target-file", seeds[2].Term)
	}
	if got, want := nodeIDs(seeds[2].Nodes), []string{"function:path"}; !reflect.DeepEqual(got, want) {
		t.Errorf("target-file seed IDs = %v, want %v", got, want)
	}
}

func TestRankUsesStableTieBreaksAndDistinctSeeds(t *testing.T) {
	nodes := []graph.Node{
		{ID: "function:z", Label: "target", QualifiedName: "src/z.ts::target", Evidence: graph.FactEvidence{Span: graph.SourceSpan{Path: "src/z.ts"}}},
		{ID: "function:a", Label: "target", QualifiedName: "src/a.ts::target", Evidence: graph.FactEvidence{Span: graph.SourceSpan{Path: "src/a.ts"}}},
		{ID: "function:b", Label: "target", QualifiedName: "src/b.ts::target", Evidence: graph.FactEvidence{Span: graph.SourceSpan{Path: "src/b.ts"}}},
		{ID: "function:duplicate", Label: "target", QualifiedName: "src/duplicate.ts::target", Evidence: graph.FactEvidence{Span: graph.SourceSpan{Path: "src/duplicate.ts"}}},
		{ID: "function:duplicate", Label: "target", QualifiedName: "src/duplicate.ts::target", Evidence: graph.FactEvidence{Span: graph.SourceSpan{Path: "src/duplicate.ts"}}},
	}

	seeds := query.Rank(nodes, []string{"target"})
	if got, want := nodeIDs(seeds[0].Nodes), []string{"function:a", "function:b", "function:duplicate"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ranked seed IDs = %v, want %v", got, want)
	}
}

func TestRankMatchesTokenAndSubstring(t *testing.T) {
	nodes := []graph.Node{
		{ID: "function:token", Label: "runner", QualifiedName: "src/task-runner.ts::run-task", Evidence: graph.FactEvidence{Span: graph.SourceSpan{Path: "src/task-runner.ts"}}},
		{ID: "function:substring", Label: "otherTarget", QualifiedName: "src/other.ts::otherTarget", Evidence: graph.FactEvidence{Span: graph.SourceSpan{Path: "src/other.ts"}}},
	}

	seeds := query.Rank(nodes, []string{"task", "hertar"})
	if got, want := nodeIDs(seeds[0].Nodes), []string{"function:token"}; !reflect.DeepEqual(got, want) {
		t.Errorf("token seed IDs = %v, want %v", got, want)
	}
	if got, want := nodeIDs(seeds[1].Nodes), []string{"function:substring"}; !reflect.DeepEqual(got, want) {
		t.Errorf("substring seed IDs = %v, want %v", got, want)
	}
}

func TestRankSnapshotReadsNodesFromOnePublishedSnapshot(t *testing.T) {
	snapshot := storage.Snapshot{Workspace: "workspace", Version: 7}
	lookup := nodeLookupFunc(func(ctx context.Context, gotSnapshot storage.Snapshot, request storage.NodeLookupRequest) ([]storage.NodeMatch, error) {
		if gotSnapshot != snapshot {
			t.Errorf("snapshot = %+v, want %+v", gotSnapshot, snapshot)
		}
		if request.Text != "main" || request.Limit <= 0 {
			t.Errorf("lookup request = %+v, want main and positive limit", request)
		}
		return []storage.NodeMatch{{Node: graph.Node{ID: "function:main", Label: "main", QualifiedName: "src/main.ts::main", Evidence: graph.FactEvidence{Span: graph.SourceSpan{Path: "src/main.ts"}}}}}, nil
	})

	seeds, err := query.RankSnapshot(context.Background(), lookup, snapshot, []string{"main"})
	if err != nil {
		t.Fatalf("rank snapshot: %v", err)
	}
	if got, want := nodeIDs(seeds[0].Nodes), []string{"function:main"}; !reflect.DeepEqual(got, want) {
		t.Errorf("snapshot seed IDs = %v, want %v", got, want)
	}
}

func TestRankSnapshotPrefersExactMatches(t *testing.T) {
	snapshot := storage.Snapshot{Workspace: "workspace", Version: 7}
	exact := graph.Node{ID: "function:main", Label: "main", QualifiedName: "src/main.ts::main"}
	exactMatches := []storage.NodeMatch{
		{Node: exact},
		{Node: graph.Node{ID: "function:other", Label: "main", QualifiedName: "src/other.ts::main"}},
		{Node: graph.Node{ID: "function:third", Label: "main", QualifiedName: "src/third.ts::main"}},
		{Node: graph.Node{ID: "function:fourth", Label: "main", QualifiedName: "src/fourth.ts::main"}},
	}
	lookup := exactNodeLookup{
		nodeLookupFunc: nodeLookupFunc(func(context.Context, storage.Snapshot, storage.NodeLookupRequest) ([]storage.NodeMatch, error) {
			t.Fatal("broad lookup is called when exact lookup finds a match")
			return nil, nil
		}),
		exact: func(_ context.Context, gotSnapshot storage.Snapshot, identifier string) ([]storage.NodeMatch, error) {
			if gotSnapshot != snapshot {
				t.Errorf("snapshot = %+v, want %+v", gotSnapshot, snapshot)
			}
			if identifier != exact.QualifiedName {
				t.Errorf("exact lookup identifier = %q, want %q", identifier, exact.QualifiedName)
			}
			return exactMatches, nil
		},
	}

	seeds, err := query.RankSnapshot(context.Background(), lookup, snapshot, []string{exact.QualifiedName})
	if err != nil {
		t.Fatalf("rank snapshot: %v", err)
	}
	if got, want := nodeIDs(seeds[0].Nodes), []string{exact.ID, "function:other", "function:third"}; !reflect.DeepEqual(got, want) {
		t.Errorf("exact seed IDs = %v, want %v", got, want)
	}
}

func TestRankSnapshotFallsBackToBroadLookupAfterExactMiss(t *testing.T) {
	snapshot := storage.Snapshot{Workspace: "workspace", Version: 7}
	broad := graph.Node{ID: "function:main", Label: "main", QualifiedName: "src/main.ts::main"}
	lookup := exactNodeLookup{
		nodeLookupFunc: nodeLookupFunc(func(_ context.Context, gotSnapshot storage.Snapshot, request storage.NodeLookupRequest) ([]storage.NodeMatch, error) {
			if gotSnapshot != snapshot {
				t.Errorf("snapshot = %+v, want %+v", gotSnapshot, snapshot)
			}
			if request.Text != "main" || request.Limit != 3 {
				t.Errorf("broad lookup request = %+v, want main with limit 3", request)
			}
			return []storage.NodeMatch{{Node: broad}}, nil
		}),
		exact: func(_ context.Context, _ storage.Snapshot, identifier string) ([]storage.NodeMatch, error) {
			if identifier != "main" {
				t.Errorf("exact lookup identifier = %q, want main", identifier)
			}
			return nil, nil
		},
	}

	seeds, err := query.RankSnapshot(context.Background(), lookup, snapshot, []string{"main"})
	if err != nil {
		t.Fatalf("rank snapshot: %v", err)
	}
	if got, want := nodeIDs(seeds[0].Nodes), []string{broad.ID}; !reflect.DeepEqual(got, want) {
		t.Errorf("broad fallback seed IDs = %v, want %v", got, want)
	}
}

type exporterFunc func(context.Context, storage.Snapshot, storage.ExportRequest, storage.ExportSink) error

func (export exporterFunc) Export(ctx context.Context, snapshot storage.Snapshot, request storage.ExportRequest, sink storage.ExportSink) error {
	return export(ctx, snapshot, request, sink)
}

type nodeLookupFunc func(context.Context, storage.Snapshot, storage.NodeLookupRequest) ([]storage.NodeMatch, error)

func (lookup nodeLookupFunc) LookupNodes(ctx context.Context, snapshot storage.Snapshot, request storage.NodeLookupRequest) ([]storage.NodeMatch, error) {
	return lookup(ctx, snapshot, request)
}

func nodeIDs(nodes []graph.Node) []string {
	ids := make([]string, len(nodes))
	for index, node := range nodes {
		ids[index] = node.ID
	}
	return ids
}
