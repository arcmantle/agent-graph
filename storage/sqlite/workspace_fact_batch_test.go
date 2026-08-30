package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"agent-atlas/extractor"
	"agent-atlas/graph"
	"agent-atlas/storage"
)

func TestContributionSessionRejectsWorkspaceFactLargerThanByteLimit(t *testing.T) {
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	store.workspaceFactBatchLimits = factBatchLimits{maximumRows: 100, maximumBytes: 64}

	ctx := context.Background()
	prior, err := store.Publish(ctx, storage.PublishRequest{Workspace: "workspace", Update: workspaceFactTestUpdate(t)})
	if err != nil {
		t.Fatalf("publish prior snapshot: %v", err)
	}

	session, err := store.BeginContributionSession(ctx, "workspace")
	if err != nil {
		t.Fatalf("begin contribution session: %v", err)
	}
	if err := session.SealContributions(ctx); err != nil {
		t.Fatalf("seal contributions: %v", err)
	}
	oversized := testNode("function:oversized", "src/oversized.ts")
	oversized.Label = strings.Repeat("x", 128)
	if err := session.WriteWorkspaceFacts(ctx, graph.Facts{Nodes: []graph.Node{oversized}}); err == nil || !strings.Contains(err.Error(), "64 byte limit") {
		t.Fatalf("write oversized workspace fact = %v, want byte-limit error", err)
	}

	current, err := store.OpenSnapshot(ctx, storage.OpenSnapshotRequest{Workspace: "workspace"})
	if err != nil {
		t.Fatalf("open current snapshot: %v", err)
	}
	if current != prior {
		t.Errorf("current snapshot after oversized fact = %+v, want prior %+v", current, prior)
	}
}

func workspaceFactTestUpdate(t *testing.T) extractor.GraphUpdate {
	t.Helper()
	vocabulary, err := graph.NewVocabulary(graph.VocabularyDefinition{NodeKinds: []graph.NodeKind{"function"}})
	if err != nil {
		t.Fatalf("create vocabulary: %v", err)
	}
	contribution, err := extractor.NewContribution(vocabulary, extractor.ContributionInput{
		ProjectID:  "project:fixture",
		SourcePath: "src/prior.ts",
		Metadata:   extractor.Metadata{Name: "typescript", Version: "1", Extensions: []string{".ts"}},
		Facts:      graph.Facts{Nodes: []graph.Node{testNode("function:prior", "src/prior.ts")}},
	})
	if err != nil {
		t.Fatalf("create contribution: %v", err)
	}
	update, err := extractor.NewGraphUpdate([]extractor.Contribution{contribution})
	if err != nil {
		t.Fatalf("create graph update: %v", err)
	}
	return update
}

func testNode(nodeID, sourcePath string) graph.Node {
	return graph.Node{
		ID:   nodeID,
		Kind: "function",
		Evidence: graph.FactEvidence{
			Span:       graph.SourceSpan{Path: sourcePath, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 2},
			FileHash:   "hash",
			Extractor:  "test@1",
			Provenance: "syntax",
			Confidence: graph.ConfidenceExtracted,
		},
	}
}
