package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"agent-graph/extractor"
	"agent-graph/graph"
	"agent-graph/storage"
	"agent-graph/storage/sqlite"

	_ "github.com/mattn/go-sqlite3"
)

func TestContributionSessionCommitPublishesOneCompleteSnapshot(t *testing.T) {
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	workspace := "workspace"
	session, err := store.BeginContributionSession(context.Background(), workspace)
	if err != nil {
		t.Fatalf("begin contribution session: %v", err)
	}

	contribution := sessionContribution(t, "src/main.ts", "function:main")
	if err := session.WriteContribution(context.Background(), contribution); err != nil {
		t.Fatalf("write contribution: %v", err)
	}

	if _, err := store.OpenSnapshot(context.Background(), storage.OpenSnapshotRequest{Workspace: workspace}); !errors.Is(err, storage.ErrWorkspaceNotFound) {
		t.Errorf("open snapshot before commit = %v, want workspace not found", err)
	}

	snapshot, err := session.Commit(context.Background(), storage.CommitRequest{})
	if err != nil {
		t.Fatalf("commit contribution session: %v", err)
	}
	if snapshot.Workspace != workspace || snapshot.Version != 1 {
		t.Errorf("committed snapshot = %+v, want workspace %q version 1", snapshot, workspace)
	}

	current, err := store.OpenSnapshot(context.Background(), storage.OpenSnapshotRequest{Workspace: workspace})
	if err != nil {
		t.Fatalf("open snapshot after commit: %v", err)
	}
	if current != snapshot {
		t.Errorf("current snapshot after commit = %+v, want %+v", current, snapshot)
	}

	target, found, err := store.ResolverTarget(context.Background(), current, extractor.ResolverTargetRequest{
		ProjectID:  "project:fixture",
		Language:   "typescript",
		SourcePath: "src/main.ts",
	})
	if err != nil {
		t.Fatalf("read resolver target: %v", err)
	}
	if !found {
		t.Fatal("resolver target for committed contribution was not found")
	}
	if !hasNodeKind(target.Nodes, "function") {
		t.Errorf("resolver target nodes = %+v, want function node", target.Nodes)
	}
}

func TestContributionSessionCommitReportsStagedTransactionMeasurement(t *testing.T) {
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	session, err := store.BeginContributionSession(context.Background(), "workspace")
	if err != nil {
		t.Fatalf("begin contribution session: %v", err)
	}
	if err := session.WriteContribution(context.Background(), sessionContribution(t, "src/main.ts", "function:main")); err != nil {
		t.Fatalf("write contribution: %v", err)
	}

	measurements := make([]storage.PublishMeasurement, 0, 2)
	_, err = session.Commit(context.Background(), storage.CommitRequest{
		Measurement: func(measurement storage.PublishMeasurement) {
			measurements = append(measurements, measurement)
		},
	})
	if err != nil {
		t.Fatalf("commit contribution session: %v", err)
	}

	wantNames := []string{"commit", "staged_transaction"}
	if len(measurements) != len(wantNames) {
		t.Fatalf("measurements = %+v, want %d measurements", measurements, len(wantNames))
	}
	for measurementIndex, want := range wantNames {
		if measurements[measurementIndex].Name != want {
			t.Errorf("measurement %d name = %q, want %q", measurementIndex, measurements[measurementIndex].Name, want)
		}
		if measurements[measurementIndex].Duration < 0 {
			t.Errorf("measurement %q duration = %s, want non-negative", want, measurements[measurementIndex].Duration)
		}
	}
	if measurements[1].Duration < measurements[0].Duration {
		t.Errorf("staged_transaction duration = %s, want at least commit duration %s", measurements[1].Duration, measurements[0].Duration)
	}
}

func TestContributionSessionRollbackLeavesPriorSnapshotCurrent(t *testing.T) {
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	workspace := "workspace"
	published, err := store.Publish(context.Background(), storage.PublishRequest{
		Workspace: workspace,
		Update:    graphUpdate(t, "src/main.ts", "function:main"),
	})
	if err != nil {
		t.Fatalf("publish initial snapshot: %v", err)
	}

	session, err := store.BeginContributionSession(context.Background(), workspace)
	if err != nil {
		t.Fatalf("begin contribution session: %v", err)
	}
	if err := session.WriteContribution(context.Background(), sessionContribution(t, "src/other.ts", "function:other")); err != nil {
		t.Fatalf("write contribution: %v", err)
	}
	if err := session.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback contribution session: %v", err)
	}

	current, err := store.OpenSnapshot(context.Background(), storage.OpenSnapshotRequest{Workspace: workspace})
	if err != nil {
		t.Fatalf("open current snapshot: %v", err)
	}
	if current != published {
		t.Errorf("current snapshot after rollback = %+v, want %+v", current, published)
	}

	_, found, err := store.ResolverTarget(context.Background(), current, extractor.ResolverTargetRequest{
		ProjectID:  "project:fixture",
		Language:   "typescript",
		SourcePath: "src/other.ts",
	})
	if err != nil {
		t.Fatalf("read resolver target: %v", err)
	}
	if found {
		t.Error("resolver target exposes rolled back contribution")
	}
}

func TestContributionSessionWriteFailureLeavesPriorSnapshotCurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graph.db")
	store, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	workspace := "workspace"
	published, err := store.Publish(context.Background(), storage.PublishRequest{
		Workspace: workspace,
		Update:    graphUpdate(t, "src/main.ts", "function:main"),
	})
	if err != nil {
		t.Fatalf("publish initial snapshot: %v", err)
	}

	database, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open verification database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`
		CREATE TRIGGER fail_session_node_batch
		BEFORE INSERT ON contribution_nodes
		WHEN NEW.node_id = 'function:fail'
		BEGIN
			SELECT RAISE(ABORT, 'injected session write failure');
		END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	session, err := store.BeginContributionSession(context.Background(), workspace)
	if err != nil {
		t.Fatalf("begin contribution session: %v", err)
	}
	if err := session.WriteContribution(context.Background(), sessionContribution(t, "src/fail.ts", "function:fail")); err == nil {
		t.Fatal("write contribution with injected failure succeeded")
	}

	current, err := store.OpenSnapshot(context.Background(), storage.OpenSnapshotRequest{Workspace: workspace})
	if err != nil {
		t.Fatalf("open current snapshot: %v", err)
	}
	if current != published {
		t.Errorf("current snapshot after write failure = %+v, want %+v", current, published)
	}
}

func TestContributionSessionCancellationKeepsPriorSnapshotCurrent(t *testing.T) {
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	workspace := "workspace"
	published, err := store.Publish(context.Background(), storage.PublishRequest{
		Workspace: workspace,
		Update:    graphUpdate(t, "src/main.ts", "function:main"),
	})
	if err != nil {
		t.Fatalf("publish initial snapshot: %v", err)
	}

	session, err := store.BeginContributionSession(context.Background(), workspace)
	if err != nil {
		t.Fatalf("begin contribution session: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := session.WriteContribution(canceled, sessionContribution(t, "src/other.ts", "function:other")); !errors.Is(err, context.Canceled) {
		t.Errorf("write canceled contribution = %v, want context cancellation", err)
	}

	current, err := store.OpenSnapshot(context.Background(), storage.OpenSnapshotRequest{Workspace: workspace})
	if err != nil {
		t.Fatalf("open current snapshot: %v", err)
	}
	if current != published {
		t.Errorf("current snapshot after cancellation = %+v, want %+v", current, published)
	}
}

func TestContributionSessionResolverReadsSeeUncommittedContributions(t *testing.T) {
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	workspace := "workspace"
	session, err := store.BeginContributionSession(context.Background(), workspace)
	if err != nil {
		t.Fatalf("begin contribution session: %v", err)
	}
	if err := session.WriteContribution(context.Background(), sessionContribution(t, "src/main.ts", "function:main")); err != nil {
		t.Fatalf("write contribution: %v", err)
	}

	target, found, err := session.ResolverTarget(context.Background(), storage.Snapshot{Workspace: workspace}, extractor.ResolverTargetRequest{
		ProjectID:  "project:fixture",
		Language:   "typescript",
		SourcePath: "src/main.ts",
	})
	if err != nil {
		t.Fatalf("read staged resolver target: %v", err)
	}
	if !found {
		t.Fatal("staged resolver target for uncommitted contribution was not found")
	}
	if !hasNodeKind(target.Nodes, "function") {
		t.Errorf("staged resolver target nodes = %+v, want function node", target.Nodes)
	}

	if _, err := session.Commit(context.Background(), storage.CommitRequest{}); err != nil {
		t.Fatalf("commit contribution session: %v", err)
	}
}

func TestContributionSessionKeepsReadersOnPriorSnapshotUntilCommit(t *testing.T) {
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	workspace := "workspace"
	published, err := store.Publish(context.Background(), storage.PublishRequest{
		Workspace: workspace,
		Update:    graphUpdate(t, "src/main.ts", "function:main"),
	})
	if err != nil {
		t.Fatalf("publish initial snapshot: %v", err)
	}

	session, err := store.BeginContributionSession(context.Background(), workspace)
	if err != nil {
		t.Fatalf("begin contribution session: %v", err)
	}
	if err := session.WriteContribution(context.Background(), sessionContribution(t, "src/other.ts", "function:other")); err != nil {
		t.Fatalf("write contribution: %v", err)
	}

	current, err := store.OpenSnapshot(context.Background(), storage.OpenSnapshotRequest{Workspace: workspace})
	if err != nil {
		t.Fatalf("open current snapshot during session: %v", err)
	}
	if current != published {
		t.Errorf("current snapshot during session = %+v, want %+v", current, published)
	}
	_, found, err := store.ResolverTarget(context.Background(), current, extractor.ResolverTargetRequest{
		ProjectID:  "project:fixture",
		Language:   "typescript",
		SourcePath: "src/other.ts",
	})
	if err != nil {
		t.Fatalf("read current resolver target during session: %v", err)
	}
	if found {
		t.Error("current snapshot exposes an uncommitted contribution")
	}

	committed, err := session.Commit(context.Background(), storage.CommitRequest{})
	if err != nil {
		t.Fatalf("commit contribution session: %v", err)
	}
	current, err = store.OpenSnapshot(context.Background(), storage.OpenSnapshotRequest{Workspace: workspace})
	if err != nil {
		t.Fatalf("open current snapshot after commit: %v", err)
	}
	if current != committed {
		t.Errorf("current snapshot after commit = %+v, want %+v", current, committed)
	}
}

func TestContributionSessionSerializesCompetingPublishers(t *testing.T) {
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	workspace := "workspace"
	if _, err := store.Publish(context.Background(), storage.PublishRequest{
		Workspace: workspace,
		Update:    graphUpdate(t, "src/main.ts", "function:main"),
	}); err != nil {
		t.Fatalf("publish initial snapshot: %v", err)
	}

	session, err := store.BeginContributionSession(context.Background(), workspace)
	if err != nil {
		t.Fatalf("begin contribution session: %v", err)
	}
	if err := session.WriteContribution(context.Background(), sessionContribution(t, "src/other.ts", "function:other")); err != nil {
		t.Fatalf("write contribution: %v", err)
	}

	type publishResult struct {
		snapshot storage.Snapshot
		err      error
	}
	started := make(chan struct{})
	finished := make(chan publishResult, 1)
	go func() {
		close(started)
		snapshot, err := store.Publish(context.Background(), storage.PublishRequest{
			Workspace: workspace,
			Update:    graphUpdate(t, "src/third.ts", "function:third"),
		})
		finished <- publishResult{snapshot: snapshot, err: err}
	}()
	<-started

	select {
	case result := <-finished:
		t.Fatalf("competing publisher finished before session commit: snapshot=%+v error=%v", result.snapshot, result.err)
	case <-time.After(100 * time.Millisecond):
	}

	committed, err := session.Commit(context.Background(), storage.CommitRequest{})
	if err != nil {
		t.Fatalf("commit contribution session: %v", err)
	}
	select {
	case result := <-finished:
		if result.err != nil {
			t.Fatalf("complete competing publisher: %v", result.err)
		}
		if result.snapshot.Version != committed.Version+1 {
			t.Errorf("competing snapshot version = %d, want %d", result.snapshot.Version, committed.Version+1)
		}
	case <-time.After(time.Second):
		t.Fatal("competing publisher did not finish after session commit")
	}
}

func sessionContribution(t *testing.T, sourcePath, nodeID string) extractor.Contribution {
	t.Helper()
	vocabulary, err := graph.NewVocabulary(graph.VocabularyDefinition{NodeKinds: []graph.NodeKind{"function"}})
	if err != nil {
		t.Fatalf("create vocabulary: %v", err)
	}
	contribution, err := extractor.NewContribution(vocabulary, extractor.ContributionInput{
		ProjectID:  "project:fixture",
		SourcePath: sourcePath,
		Metadata:   extractor.Metadata{Name: "typescript", Version: "1", Extensions: []string{".ts"}},
		Facts: graph.Facts{Nodes: []graph.Node{{
			ID:   nodeID,
			Kind: "function",
			Evidence: graph.FactEvidence{
				Span:       graph.SourceSpan{Path: sourcePath, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 5},
				FileHash:   "content-hash",
				Extractor:  "typescript@1",
				Provenance: "syntax",
				Confidence: graph.ConfidenceExtracted,
			},
		}}},
	})
	if err != nil {
		t.Fatalf("create contribution: %v", err)
	}
	return contribution
}
