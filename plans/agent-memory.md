# Plan: agent-memory

> Status: **All phases complete**

## Architectural decisions

- **Path Schema**: `~/.config/agent-broker/` for the database.
- **Database**: SQLite in WAL mode with FTS5, using `github.com/mattn/go-sqlite3`.
- **MemoryService (Session Factory)**: The `MemoryService` interface returns a `Session` object. `Session` methods (`Log`, `Ask`, `Lock`, `Compact`, `Export`) hide session-id plumbing.
- **Locks**: SQLite-backed with TTL (1-hour default), cross-process visible.
- **Compaction**: Archive old entries in-place, search filters `archived=0`.
- **Export**: On-demand Markdown from SQLite — no background syncing.
- **Privacy (Mandatory Middleware)**: `.mcpignore` file patterns checked in `pkg/service` before persistence.
- **Interfaces**: MCP (via `github.com/mark3labs/mcp-go`) and REST (via standard library `net/http`) act as "Dumb Transports."
- **CLI Framework**: `github.com/spf13/cobra`.

## Phase 1: Deep Storage Foundations

**Status: ✅ Complete**

Session factory, SQLite persistence with WAL mode, and FTS5 search.

### Acceptance criteria
- [x] `MemoryService.Connect(id)` returns a valid `Session` object.
- [x] `Session.Log(entry)` commits to SQLite.
- [x] Markdown export available via on-demand command (no background syncing).
- [x] Verified: Appends do not block the caller (synchronous, no channel).

## Phase 2: The Dumb Transport (MCP via Service)

**Status: ✅ Complete**

MCP server using `mcp-go`. The broker is a thin, logic-free wrapper.

### Acceptance criteria
- [x] `agent-mem start --session <id>` launches the MCP server.
- [x] MCP tool calls are routed to the `MemoryService`.
- [x] The broker contains zero business logic.

## Phase 3: Fast Retrieval (FTS5 Search)

**Status: ✅ Complete**

Retrieval engine using SQLite FTS5 with keyword matching and stemming.

### Acceptance criteria
- [x] `search_memory` tool is available via MCP.
- [x] The tool returns relevant log entries based on keyword matching via FTS5.
- [x] Search results exclude archived entries by default.

## Phase 4: Multi-Agent Safety (Advisory Locking)

**Status: ✅ Complete**

SQLite-backed locking manager with TTL, visible across processes.

### Acceptance criteria
- [x] Agents can claim a lock on a file path with TTL.
- [x] If Agent A holds a lock, Agent B's request to acquire it is rejected.
- [x] Locks can be released via `release_lock` tool.
- [x] Lock states are persistent across agent restarts (stored in DB).
- [x] Expired locks are automatically cleaned on read.

## Phase 5: The "Shim" & Privacy

**Status: ✅ Complete**

REST API and `.mcpignore` privacy guardrails.

### Acceptance criteria
- [x] A local HTTP server responds to all session operations (log, search, lock, export, compact).
- [x] The broker rejects operations involving patterns defined in `.mcpignore`.
- [x] Sensitive patterns (API keys, secrets) are blocked from being appended to logs.

## Phase 6: Orchestration & Polishing

**Status: ✅ Complete**

CLI orchestration layer and cross-repository session sharing.

### Acceptance criteria
- [x] `agent-mem list` shows sessions with entry counts.
- [x] `agent-mem prune` cleans up archived entries older than N days.
- [x] `agent-mem stop` stops the REST server.
- [x] Verified: Two agents in different directories using the same Session ID see each other's updates (shared SQLite).
