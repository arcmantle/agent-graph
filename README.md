# agent-graph

`agent-graph` is a local-first, continuously updated code graph for AI agents.

It is intended to work beside `agent-issues`:

- `agent-issues` records work state, plans, dependencies, and decisions.
- `agent-graph` records code structure, relationships, change impact, and architecture context.

The goal is to make broad codebase questions fast and evidence-based without replacing source inspection, language-server navigation, tests, or Git.

## Why This Exists

Graphify demonstrates the value of a code graph for AI workflows, but its JSON-first design is not ideal for a continuously updated monorepo index:

- Queries load a full graph JSON document and scan nodes before traversal.
- Graph outputs are useful interchange artifacts but weak primary storage.
- Per-package graphs require separate updates and a merge step.
- Reports and visualization do not need to update on every edit.

`agent-graph` will use Graphify as a behavioral reference while taking a database-first, Go-native approach to indexing and updates.

## Product Principles

- Local-first SQLite storage, with optional deployed PostgreSQL.
- One storage contract and migration model for SQLite and PostgreSQL.
- `graph.json` is an export and snapshot format, not the query store.
- Source code remains authoritative. The graph is retrieval and planning evidence.
- Every extracted fact records source path, source location, content hash, extractor version, timestamp, and confidence.
- Exact lookup stays with LSP and text search. The graph serves architecture, dependency, impact, and data-flow questions.
- Generated reports and HTML are delayed or explicit work, not part of the edit hot path.

## Core Design

### Extractor Packages

Language extractors are statically linked, isolated Go packages. Each package
under `extractors/<language>` owns one language's parser integration, file
matching, version, and local-fact mapping. `extractors/registry` is the single
composition point that imports and registers those packages. Extractors depend
on shared contracts only; they do not depend on each other or on the registry.

v0 uses the Go Tree-sitter runtime with JavaScript and TypeScript grammar
bindings. Local checks and CI require a C compiler and run with `CGO_ENABLED=1`.

### Storage

The initial relational model should include:

```text
projects
files
file_versions
nodes
edges
extractions
graph_snapshots
query_results
```

Required indexes include node labels, file references, and both edge endpoints. SQLite FTS5 will support local node lookup. PostgreSQL full-text search can provide the deployed equivalent. Vector search is optional and should not be required for the first version.

### Continuous Indexing

A long-running Go service watches project files, normalizes events, and debounces bursts from editors.

```text
filesystem event
  -> normalize and debounce paths
  -> identify added, changed, and deleted files
  -> extract affected files
  -> transactionally replace file nodes and edges
  -> publish a new queryable graph version
```

Use a periodic reconciliation scan to recover from missed or reordered filesystem events.

Target timings:

- File extraction and graph update: debounced at about 250-1000 ms.
- Clustering, metrics, reports, and HTML: after 30-60 seconds of inactivity, or by explicit command.

### Querying

Queries should use indexed node lookup followed by bounded graph traversal:

```text
question terms
  -> full-text node search
  -> ranked start nodes
  -> indexed incoming and outgoing edge traversal
  -> evidence with paths, locations, and confidence
```

Support commands such as:

```text
agent-graph index
agent-graph watch
agent-graph query "How does X reach Y?"
agent-graph path X Y
agent-graph explain X
agent-graph report
agent-graph export
```

## Relationship To AI Workflows

An AI agent should use `agent-graph` to find relevant code areas and relationships for architecture, dependency, call-flow, and impact questions. It must inspect the current source before edits and use tests and Git diff for verification.

The graph can be stale between index updates. Query results must identify the graph version and source evidence.

## Reference Implementation

The `reference/` directory contains a shallow clone of Graphify at the time this project was created. It is a behavioral oracle, not a code template.

Use it to create golden fixtures and compare:

- node and edge identities
- source locations and confidence metadata
- extraction output
- incremental replacement and deletion behavior
- query, path, and explain results

Do not port Python files one-for-one. Port externally visible behavior through small, tested vertical slices.

## Initial Delivery Plan

1. Define the Go module, storage interface, SQLite implementation, and schema migrations.
2. Extract TypeScript and JavaScript fixtures into stable nodes and edges.
3. Add indexed `query`, `path`, and `explain` commands.
4. Replace changed-file graph facts and remove deleted-file graph facts transactionally.
5. Add a workspace-level index so one update covers all configured packages.
6. Add a file watcher with debounce, queueing, and reconciliation.
7. Add delayed clustering, reports, HTML, and JSON exports.
8. Add optional semantic extraction and PostgreSQL support.

## Out Of Scope For The First Version

- Image, video, and audio extraction.
- Required cloud LLM access.
- Vector search as the main retrieval method.
- Instant report regeneration on every file edit.
- Full export parity before the core index and update loop is reliable.

## Validation Strategy

The Go implementation should use the Python reference as a behavioral oracle. For each fixture, compare normalized graph JSON and query results. Test SQLite by default and run the same storage contract tests against PostgreSQL in CI.

## Release Validation

Run the supported v0 release gate with:

```bash
make acceptance
```

This command runs the storage conformance, extraction, indexing, query, path, explain, export, lifecycle, fixture, and Graphify comparison checks.
