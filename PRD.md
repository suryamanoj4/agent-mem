# PRD: agent-memory

## Problem Statement

Developers using multiple coding agents face "context fragmentation." Each agent operates in isolated memory spaces, leading to redundant instructions, conflicting architectural decisions, and loss of context when agent sessions end.

## Solution

`agent-memory` is a persistent shared blackboard for coding agents. It runs as a local daemon and provides a cross-repository memory layer that is air-gapped, human-readable (via on-demand Markdown export), and requires zero model-specific code.

## User Stories

1. ✅ As a developer, I want to initialize a named session (e.g., `feature-auth`), so that I can group the work of multiple agents under one context.
2. ✅ As a developer, I want my agents to connect to the broker via MCP, so that they can automatically discover tools for reading and writing memory.
3. ✅ As a developer, I want to use custom scripts to interact with the broker via a REST API, so that I can integrate non-MCP-native tools.
4. ✅ As an agent, I want to append a log of my decisions to the session, so that other agents in the same session can stay informed.
5. ✅ As an agent, I want to search the project memory using natural language keywords, so that I can retrieve relevant past decisions.
5a. ✅ As an agent, I want search to return surrounding context (prior and subsequent entries in the session thread), so that I see full conversation threads instead of isolated matching lines.
6. ✅ As an agent, I want to check the lock status of a file before editing it, so that I don't conflict with another agent working on the same file.
7. ✅ As a developer, I want to export memory as Markdown on demand, so that I can audit the logs using standard text editors.
8. ✅ As a developer, I want the broker to respect `.mcpignore` files, so that sensitive information like API keys is never ingested into the memory.
9. ✅ As a developer, I want to run agents in different repositories and have them share the same session, so that they maintain architectural consistency.

## Architecture

### Modules

- **`cmd/agent-mem`**: CLI entrypoint for session orchestration (`start`, `serve`, `export`, `list`, `prune`, `stop`).
- **`pkg/service`**: Session factory with advisory locking, privacy filtering, and search delegation.
- **`pkg/store`**: Manages SQLite/FTS5 persistence and on-demand Markdown export.
- **`pkg/privacy`**: `.mcpignore` pattern matching for content filtering.
- **`pkg/broker`**: Pure transport wrappers — MCP (stdio) and REST (HTTP).

### Key Decisions

- **Language:** Go
- **Storage:** SQLite in WAL mode with FTS5 for keyword search
- **Locks:** SQLite-backed with TTL, visible across processes
- **Compaction:** Archive old entries, search excludes archived by default, export includes all
- **Export:** On-demand Markdown generation from SQLite
- **Transport:** MCP over stdio for agents, REST over HTTP for scripts

## Testing

- **Behavioral Testing:** Tests cover the full append → search → lock → compact → export lifecycle.
- **Cross-Process Testing:** Tests verify lock state is shared across separate store instances.
- **Privacy Filtering:** Unit tests for `.mcpignore` pattern matching.

## Out of Scope

- Multi-machine synchronization (memory is strictly local)
- Built-in TUI for reading logs (use preferred Markdown viewers)
- Automatic repository detection (sessions must be explicitly named)
