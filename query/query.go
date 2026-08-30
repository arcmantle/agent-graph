package query

import (
	"context"
	"fmt"

	"agent-atlas/graph"
	"agent-atlas/storage"
)

type Request struct {
	Terms      []string
	ProjectIDs []string
	Relations  []graph.RelationKind
	MaxDepth   int
	MaxNodes   int
}

type Result struct {
	Seeds             []SeedSet
	Facts             graph.Facts
	TruncationReasons []storage.TruncationReason
	ScopeBoundary     *ScopeBoundary
}

func QuerySnapshot(ctx context.Context, lookup storage.NodeLookup, traverser storage.Traverser, snapshot storage.Snapshot, request Request) (Result, error) {
	if lookup == nil {
		return Result{}, fmt.Errorf("query published graph: node lookup is required")
	}
	if traverser == nil {
		return Result{}, fmt.Errorf("query published graph: traverser is required")
	}
	if len(request.Terms) == 0 || request.MaxDepth < 0 || request.MaxNodes <= 0 {
		return Result{}, fmt.Errorf("query published graph: terms, nonnegative maximum depth, and positive maximum nodes are required")
	}

	seeds, err := RankSnapshot(ctx, lookup, snapshot, request.Terms)
	if err != nil {
		return Result{}, fmt.Errorf("query published graph: %w", err)
	}
	startNodeIDs := make([]string, 0, len(seeds)*maxSeedsPerTerm)
	for _, seedSet := range seeds {
		for _, node := range seedSet.Nodes {
			startNodeIDs = append(startNodeIDs, node.ID)
		}
	}
	if len(startNodeIDs) == 0 {
		return Result{Seeds: seeds}, nil
	}

	traversal, err := traverser.Traverse(ctx, snapshot, storage.TraversalRequest{
		StartNodeIDs: startNodeIDs,
		ProjectIDs:   append([]string(nil), request.ProjectIDs...),
		Direction:    storage.TraverseOutgoing,
		Relations:    append([]graph.RelationKind(nil), request.Relations...),
		MaxDepth:     request.MaxDepth,
		MaxNodes:     request.MaxNodes,
	})
	if err != nil {
		return Result{}, fmt.Errorf("query published graph: %w", err)
	}
	var boundary *ScopeBoundary
	if traversal.ScopeBoundary != nil {
		boundary = &ScopeBoundary{Node: *traversal.ScopeBoundary}
	}
	return Result{
		Seeds:             seeds,
		Facts:             traversal.Facts,
		TruncationReasons: traversal.TruncationReasons,
		ScopeBoundary:     boundary,
	}, nil
}
