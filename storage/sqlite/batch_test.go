package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestInsertRowsAdaptsToSQLiteVariableLimit(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("start transaction: %v", err)
	}
	t.Cleanup(func() { _ = transaction.Rollback() })
	if _, err := transaction.ExecContext(ctx, "CREATE TEMP TABLE batch_fixture (name TEXT NOT NULL, value INTEGER NOT NULL)"); err != nil {
		t.Fatalf("create batch fixture: %v", err)
	}

	rows := [][]any{{"one", 1}, {"two", 2}, {"three", 3}, {"four", 4}, {"five", 5}}
	if err := insertRows(ctx, transaction, 6, 500, "INSERT INTO batch_fixture (name, value) VALUES ", rows); err != nil {
		t.Fatalf("insert adaptive batches: %v", err)
	}

	var count int
	if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM batch_fixture").Scan(&count); err != nil {
		t.Fatalf("count inserted rows: %v", err)
	}
	if count != len(rows) {
		t.Errorf("inserted row count = %d, want %d", count, len(rows))
	}
}

func TestPublicationWorkerCountStaysWithinContributionAndConfiguredBounds(t *testing.T) {
	for _, contributionCount := range []int{0, 1, 2, maximumPublicationWorkers + 1} {
		workerCount := publicationWorkerCount(contributionCount)
		if workerCount > contributionCount {
			t.Errorf("worker count for %d contributions = %d, exceeds contribution count", contributionCount, workerCount)
		}
		if workerCount > maximumPublicationWorkers {
			t.Errorf("worker count for %d contributions = %d, exceeds configured maximum %d", contributionCount, workerCount, maximumPublicationWorkers)
		}
	}
}
