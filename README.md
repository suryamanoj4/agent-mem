# agent-memory

**agent-memory** is a persistent context broker for multi-agent coding workflows. It provides a shared blackboard where agents write decisions and read what other agents in the same session have decided — surviving beyond individual agent sessions.

Built in Go, it exposes tools via the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) for agent integration and a REST API for custom scripts.

## Problem

In a multi-agent workflow, agents suffer from "context fragmentation" — the frontend agent doesn't know what the backend agent decided, leading to conflicting approaches. Each agent's context window resets between sessions, losing critical architectural decisions.

## Solution

`agent-memory` is a persistent shared blackboard. Agents append decisions to a named session, and other agents in the same session search those decisions via natural language keywords. The blackboard survives agent restarts — the context that normally gets lost persists in SQLite.

## Architecture

- **Core Engine (MemoryService)**: Session factory and business logic layer.
- **Storage**: SQLite in WAL mode with FTS5 for keyword search.
- **Advisory Locking**: SQLite-backed locks with TTL, visible across multiple agent processes.
- **Compaction**: Agents can archive old entries to keep search results focused on recent decisions. Archived entries remain visible in export with `[archived]` tags.
- **Privacy**: `.mcpignore` file patterns block sensitive content from being stored.
- **Export**: On-demand Markdown generation from SQLite — no background syncing.
- **Interfaces**:
  - **MCP**: Native integration for coding agents (6 tools).
  - **REST**: HTTP API for custom scripts on localhost.

## Commands

| Command | Description |
|---------|-------------|
| `agent-mem start --session <id> [--mcpignore .mcpignore]` | Start MCP broker for a session |
| `agent-mem serve --port 4096` | Start REST API server for all sessions |
| `agent-mem export --session <id>` | Export session memory as Markdown to stdout |
| `agent-mem list` | List all sessions with entry counts |
| `agent-mem prune --days 30` | Delete archived entries older than N days |
| `agent-mem stop` | Stop the running REST server |

## MCP Tools

| Tool | Description |
|------|-------------|
| `append_log` | Append a decision to session memory |
| `search_memory` | Search session memory via FTS5 keyword search |
| `acquire_lock` | Lock a file path (with 1-hour TTL) |
| `release_lock` | Release a lock on a file path |
| `get_lock_status` | Check if a file path is locked |
| `compact_session` | Archive all non-archived entries |

## REST Endpoints

```
POST   /api/sessions/{id}/log
GET    /api/sessions/{id}/search?query=...
POST   /api/sessions/{id}/lock
DELETE /api/sessions/{id}/lock?path=...
GET    /api/sessions/{id}/lock/status?path=...
POST   /api/sessions/{id}/compact
GET    /api/sessions/{id}/export
```

## Project Structure

```
cmd/agent-mem/       CLI entrypoint (start, serve, export, list, prune, stop)
pkg/service/         Session factory, privacy filter, lock delegation
pkg/broker/          MCP transport + REST transport
pkg/store/           SQLite persistence (WAL + FTS5 + locks)
pkg/privacy/         .mcpignore content filtering
pkg/api/             Shared interfaces and types
```

## Quick Start

```bash
# Start a session for an agent
agent-mem start --session feature-auth

# In another terminal, start REST server
agent-mem serve --port 4096

# List all sessions
agent-mem list

# Export a session as Markdown
agent-mem export --session feature-auth > feature-auth.md
```

### Prerequisites
- Go 1.26+
- SQLite3 with FTS5 support (build with `-tags sqlite_fts5`)

## Docs
- [PRD.md](./PRD.md): Product Requirements Document.
- [docs/agent-memory.md](./docs/agent-memory.md): Implementation Roadmap.
- [docs/decision-store-ports-adapters.md](./docs/decision-store-ports-adapters.md): Decision Store Refactor Plan.

## License
MIT
