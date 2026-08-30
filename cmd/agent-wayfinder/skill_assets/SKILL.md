---
name: agent-wayfinder
description: "Use for codebase architecture, dependency, call-flow, impact, and project-relationship questions, and for Agent Wayfinder install, index, query, path, explain, export, or indexer tasks. Query an existing local code graph before broad source search, then verify graph evidence in current source."
argument-hint: "[question, node, path, or indexing task]"
---

# Agent Wayfinder

Use Agent Wayfinder as a local map of code structure and relationships. The graph is retrieval and planning evidence. Source code, language-server results, tests, and Git remain authoritative.

## Usage

```text
/agent-wayfinder install [--project]
/agent-wayfinder index [WORKSPACE]
/agent-wayfinder query [WORKSPACE] <question or terms>
/agent-wayfinder path [WORKSPACE] <SOURCE> <TARGET>
/agent-wayfinder explain [WORKSPACE] <NODE>
/agent-wayfinder export [WORKSPACE]
/agent-wayfinder indexer <serve|start|status|stop> [WORKSPACE]
```

If the user gives no workspace, use the repository root. If there is no repository root, use the current directory.

## Choose A Command

| Need | Command |
|---|---|
| Install this skill for the user or current project | `install` |
| Build or refresh the graph | `index` |
| Find relevant nodes and nearby outgoing relationships | `query` |
| Trace a directed relationship chain | `path` |
| Inspect one exact or unambiguous node | `explain` |
| Read the complete published graph | `export` |
| Control the current indexer process | `indexer` |

Load [the command reference](./references/commands.md) when you need exact syntax, flags, installation details, or indexer limits.

## Procedure

1. Resolve the workspace to an absolute repository root.
2. Prefer `agent-wayfinder`. Use `a-wayfinder` if only that alias is installed. In the Agent Wayfinder source repository, use `go run ./cmd/agent-wayfinder` when no installed command is available.
3. Check for `<WORKSPACE>/.agent-wayfinder/graph.db`, unless the user gave `--database`.
4. If no published graph is available, run `index WORKSPACE --format json` before a graph query.
5. For questions about recent edits, refresh with `index`. Do not assume that the current `indexer start` command updates the graph.
6. Select focused query terms from identifiers, qualified names, file names, and source paths in the user's question. Pass each term as a separate argument. Do not pass a full natural-language sentence as one term.
7. Use `--format json` for agent work. Read `graphVersion`, `publishedAt`, source evidence, scope boundaries, and truncation reasons.
8. Inspect the cited source before an edit or a final technical claim. Use language-server navigation or text search for exact symbol lookup.
9. Run focused tests and inspect the Git diff after a change. A graph result is not validation.

## Query Fast Path

When a published graph exists and the user asks a broad codebase question, query it before broad file search:

```bash
agent-wayfinder query "$WORKSPACE" AuthService token src/auth --format json
```

Start with exact identifiers or paths. If there are no useful seeds, retry with shorter identifier prefixes or individual path terms. If results remain empty, use source search and state that the graph did not contain useful evidence.

Use bounded traversal. Increase `--max-depth` or `--max-nodes` only when the result reports truncation or the first traversal stops before the needed boundary.

## Path And Explain

Use `path` when both endpoints are known:

```bash
agent-wayfinder path "$WORKSPACE" AuthService TokenStore --format json
```

Paths are directed by default. Use `--undirected` only as an explicit fallback, and state that the result does not prove directed flow.

Use `explain` for one node:

```bash
agent-wayfinder explain "$WORKSPACE" AuthService --format json
```

If `explain` returns several candidates, choose from those candidates and rerun with the exact node ID. Do not select an ambiguous node from memory.

## Evidence Rules

- Cite graph evidence with its source path and location when available.
- State the graph version and publication time when freshness matters.
- Treat scope boundaries and truncation as limits on the answer.
- Do not invent missing edges or treat an undirected fallback as call direction.
- Do not read the complete export when a bounded query, path, or explanation can answer the question.