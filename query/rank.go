package query

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"agent-wayfinder/graph"
	"agent-wayfinder/storage"
)

const maxSeedsPerTerm = 3

type SeedSet struct {
	Term  string
	Nodes []graph.Node
}

type ExplainResult struct {
	Candidates     []graph.Node
	RemainderCount int
	Explanation    *storage.Explanation
}

func (result ExplainResult) CandidateIDs() []string {
	ids := make([]string, len(result.Candidates))
	for index, candidate := range result.Candidates {
		ids[index] = candidate.ID
	}
	return ids
}

func Rank(nodes []graph.Node, terms []string) []SeedSet {
	seeds := make([]SeedSet, 0, len(terms))
	for _, term := range terms {
		seeds = append(seeds, SeedSet{Term: term, Nodes: rankTerm(nodes, term)})
	}
	return seeds
}

func RankSnapshot(ctx context.Context, lookup storage.NodeLookup, snapshot storage.Snapshot, terms []string) ([]SeedSet, error) {
	if lookup == nil {
		return nil, fmt.Errorf("rank published graph: node lookup is required")
	}

	seeds := make([]SeedSet, 0, len(terms))
	for _, term := range terms {
		matches, err := lookupTermMatches(ctx, lookup, snapshot, term, maxSeedsPerTerm)
		if err != nil {
			return nil, fmt.Errorf("rank published graph: %w", err)
		}
		nodes := make([]graph.Node, len(matches))
		for index, match := range matches {
			nodes[index] = match.Node
		}
		seeds = append(seeds, SeedSet{Term: term, Nodes: nodes})
	}
	return seeds, nil
}

func ExplainSnapshot(ctx context.Context, lookup storage.NodeLookup, explainer storage.Explainer, snapshot storage.Snapshot, term string) (ExplainResult, error) {
	if explainer == nil {
		return ExplainResult{}, fmt.Errorf("explain published graph: explainer is required")
	}
	if lookup == nil {
		return ExplainResult{}, fmt.Errorf("explain published graph: node lookup is required")
	}
	matches, err := lookupTermMatches(ctx, lookup, snapshot, term, maxSeedsPerTerm+1)
	if err != nil {
		return ExplainResult{}, fmt.Errorf("explain published graph: %w", err)
	}
	candidates := make([]graph.Node, len(matches))
	for index, match := range matches {
		candidates[index] = match.Node
	}
	if len(candidates) != 1 {
		return ExplainResult{
			Candidates:     candidates[:min(len(candidates), maxSeedsPerTerm)],
			RemainderCount: max(0, len(candidates)-maxSeedsPerTerm),
		}, nil
	}
	explanation, err := explainer.Explain(ctx, snapshot, storage.ExplainRequest{NodeID: candidates[0].ID})
	if err != nil {
		return ExplainResult{}, fmt.Errorf("explain published graph: %w", err)
	}
	return ExplainResult{Explanation: &explanation}, nil
}

func lookupTermMatches(ctx context.Context, lookup storage.NodeLookup, snapshot storage.Snapshot, term string, limit int) ([]storage.NodeMatch, error) {
	if exactLookup, supported := lookup.(storage.ExactNodeLookup); supported {
		matches, err := exactLookup.LookupExactNodes(ctx, snapshot, term)
		if err != nil {
			return nil, err
		}
		if len(matches) > 0 {
			return matches[:min(len(matches), limit)], nil
		}
	}
	return lookup.LookupNodes(ctx, snapshot, storage.NodeLookupRequest{Text: term, Limit: limit})
}

type nodeCollector struct {
	nodes []graph.Node
}

func (collector *nodeCollector) WriteNode(node graph.Node) error {
	collector.nodes = append(collector.nodes, node)
	return nil
}

func (*nodeCollector) WriteEdge(graph.Edge) error {
	return nil
}

type rankedNode struct {
	node graph.Node
	rank int
}

func rankTerm(nodes []graph.Node, term string) []graph.Node {
	matches := rankTermMatches(nodes, term)
	return matches[:min(len(matches), maxSeedsPerTerm)]
}

func rankTermMatches(nodes []graph.Node, term string) []graph.Node {
	if term == "" {
		return nil
	}

	matches := make([]rankedNode, 0, len(nodes))
	for _, node := range nodes {
		rank, matched := matchRank(node, term)
		if matched {
			matches = append(matches, rankedNode{node: node, rank: rank})
		}
	}
	sort.Slice(matches, func(left, right int) bool {
		if matches[left].rank != matches[right].rank {
			return matches[left].rank < matches[right].rank
		}
		if matches[left].node.QualifiedName != matches[right].node.QualifiedName {
			return matches[left].node.QualifiedName < matches[right].node.QualifiedName
		}
		if matches[left].node.Label != matches[right].node.Label {
			return matches[left].node.Label < matches[right].node.Label
		}
		return matches[left].node.ID < matches[right].node.ID
	})

	selected := make([]graph.Node, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if _, found := seen[match.node.ID]; found {
			continue
		}
		seen[match.node.ID] = struct{}{}
		selected = append(selected, match.node)
	}
	return selected
}

func matchRank(node graph.Node, term string) (int, bool) {
	if node.ID == term {
		return 0, true
	}
	if node.QualifiedName == term {
		return 1, true
	}
	if node.Label == term {
		return 2, true
	}

	query := strings.ToLower(term)
	if strings.HasPrefix(strings.ToLower(node.Label), query) || hasTokenPrefix(node.Label, query) || hasTokenPrefix(node.QualifiedName, query) {
		return 3, true
	}
	if strings.Contains(strings.ToLower(node.Evidence.Span.Path), query) {
		return 4, true
	}
	if strings.Contains(strings.ToLower(node.ID), query) || strings.Contains(strings.ToLower(node.QualifiedName), query) || strings.Contains(strings.ToLower(node.Label), query) {
		return 5, true
	}
	return 0, false
}

func hasTokenPrefix(value, term string) bool {
	for _, token := range strings.FieldsFunc(value, func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character)
	}) {
		if strings.HasPrefix(strings.ToLower(token), term) {
			return true
		}
	}
	return false
}
