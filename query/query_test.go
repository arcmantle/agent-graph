package query_test

import (
	"context"
	"testing"

	"agent-atlas/graph"
	"agent-atlas/query"
	"agent-atlas/storage"
)

func TestQuerySnapshotReportsExternalProjectScopeBoundary(t *testing.T) {
	snapshot := storage.Snapshot{Workspace: "workspace", Version: 7}
	main := graph.Node{ID: "function:main", Label: "main", QualifiedName: "apps/app/src/main.ts::main"}
	helper := graph.Node{ID: "function:helper", Label: "helper", QualifiedName: "packages/library/src/helper.ts::helper"}
	lookup := nodeLookupFunc(func(_ context.Context, _ storage.Snapshot, request storage.NodeLookupRequest) ([]storage.NodeMatch, error) {
		if request.Text != "main" || request.Limit <= 0 {
			t.Errorf("lookup request = %+v, want main and positive limit", request)
		}
		return []storage.NodeMatch{{Node: main}}, nil
	})
	traverser := traverserFunc(func(_ context.Context, _ storage.Snapshot, request storage.TraversalRequest) (storage.TraversalResult, error) {
		return storage.TraversalResult{Facts: graph.Facts{
			Nodes: []graph.Node{main, helper},
			Edges: []graph.Edge{{SourceID: main.ID, TargetID: helper.ID, Relation: "calls"}},
		}, ScopeBoundary: &helper}, nil
	})

	result, err := query.QuerySnapshot(context.Background(), lookup, traverser, snapshot, query.Request{
		Terms:      []string{"main"},
		ProjectIDs: []string{"project:app"},
		MaxDepth:   2,
		MaxNodes:   3,
	})
	if err != nil {
		t.Fatalf("query published graph: %v", err)
	}
	if result.ScopeBoundary == nil || result.ScopeBoundary.Node.ID != helper.ID {
		t.Errorf("scope boundary = %+v, want external helper", result.ScopeBoundary)
	}
}
