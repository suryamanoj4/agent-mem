# agent-memory

**agent-memory** is a shared decision store for multi-agent coding workflows. Agents store structured decisions (architecture, code changes, plans, preferences) and retrieve what other agents decided — surviving beyond individual agent sessions.

Built in Go, it exposes tools via the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) for agent integration and a REST API for custom scripts.

## Problem

In a multi-agent workflow, agents suffer from "context fragmentation" — the frontend agent doesn't know what the backend agent decided, leading to conflicting approaches. Developers end up copy-pasting messages between agent sessions.

## Solution

`agent-memory` is a persistent decision store. Agents store structured decisions with agent attribution, decision types, and optional diffs. Other agents retrieve cross-agent context — seeing what other agents decided plus user preferences — without needing to wade through raw conversation history.

## Architecture

- **Decision Store**: SQLite in WAL mode with FTS5 for full-text search.
- **Attribution**: Every decision has agent_id + author_type (agent/user) + decision_type (architecture, code_change, plan, preference, note).
- **Cross-Agent Context**: `get_context` excludes the caller's own decisions, returns decisions from other agents + user preferences.
- **Locking**: SQLite-backed locks with TTL, visible across processes.
- **Export**: On-demand Markdown from SQLite — no background syncing.
- **Interfaces**:
  - **MCP**: 8 tools for agent integration.
  - **REST**: HTTP API for custom scripts.

## Commands

| Command | Description |
|---------|-------------|
| `agent-mem start --session <id>` | Start MCP broker for a session |
| `agent-mem serve --port 4096` | Start REST API server for all sessions |
| `agent-mem export --session <id>` | Export session decisions as Markdown |
| `agent-mem list` | List all sessions with decision counts |
| `agent-mem prune --days 30` | Delete archived decisions older than N days |
| `agent-mem stop` | Stop the running REST server |

## MCP Tools

| Tool | Description |
|------|-------------|
| `decide` | Store a decision (architecture, code_change, plan, note) |
| `get_context` | Get decisions from other agents + user preferences |
| `prefer` | Store a user preference or instruction |
| `search_decisions` | Search decisions by keyword, filter by type |
| `acquire_lock` | Lock a file path (1-hour TTL) |
| `release_lock` | Release a lock on a file path |
| `get_lock_status` | Check if a file path is locked |
| `compact_session` | Archive all non-archived decisions |

## REST Endpoints

```
POST   /api/sessions/{id}/decide
GET    /api/sessions/{id}/context?agent_id=...
POST   /api/sessions/{id}/prefer
GET    /api/sessions/{id}/search?query=...&type=...
POST   /api/sessions/{id}/lock
DELETE /api/sessions/{id}/lock?path=...
GET    /api/sessions/{id}/lock/status?path=...
POST   /api/sessions/{id}/compact
GET    /api/sessions/{id}/export
```

## Project Structure

```
cmd/agent-mem/       CLI entrypoint (start, serve, export, list, prune, stop)
pkg/decision/        Domain types, ports, SQLite adapter, in-memory test adapter
pkg/broker/          MCP transport + REST transport
pkg/privacy/         .mcpignore content filtering
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
