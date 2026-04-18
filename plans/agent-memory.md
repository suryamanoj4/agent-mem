# Plan: agent-memory

> Source PRD: ./PRD.md

## Architectural decisions

Durable decisions that apply across all phases:

- **Path Schema**: `~/.config/agent-broker/` for the database and `sessions/<session_id>/` for Markdown logs.
- **Database**: SQLite in WAL mode with FTS5 enabled, using `github.com/mattn/go-sqlite3`.
- **MemoryService (Session Factory)**: The `MemoryService` interface returns a `Session` object. `Session` methods (`Log`, `Ask`, `Lock`) hide the session-id plumbing.
- **Shadow Sync (Background Buffer)**: The `pkg/store` uses a Go channel and background worker to asynchronously mirror SQLite writes to Markdown logs.
- **Privacy (Interceptor Middleware)**: A mandatory internal layer in `pkg/service` that scrubs payloads using `github.com/go-git/go-git/v5` ignore logic before persistence.
- **Interfaces**: MCP (via `github.com/mark3labs/mcp-go`) and REST (via `github.com/go-chi/chi/v5`) act as "Dumb Transports."
- **CLI Framework**: `github.com/spf13/cobra` for robust subcommand management.

---

## Phase 1: Deep Storage Foundations (Service + Store + Sync)

**User stories**: 
- 1. As a developer, I want to initialize a named session...
- 4. As an agent, I want to append a log of my decisions...
- 7. As a developer, I want the memory to be stored as Markdown files...

### What to build

The core "Deep Brain": `pkg/service` and `pkg/store`. This phase implements the **Session Factory** interface and the **Background Buffer** for Markdown mirroring. It establishes the SQLite schema and the background synchronization worker.

### Acceptance criteria

- [ ] `MemoryService.Connect(id)` returns a valid `Session` object.
- [ ] `Session.Log(entry)` successfully commits to SQLite via `pkg/store`.
- [ ] The background worker asynchronously mirrors the log entry to the correct `.md` file.
- [ ] Verified: High-frequency appends do not block the main execution thread.

---

## Phase 2: The Dumb Transport (MCP via Service)

**User stories**:
- 2. As a developer, I want my agents to connect to the broker via MCP...
- 4. As an agent, I want to append a log of my decisions...

### What to build

The MCP server implementation using `mcp-go`. In this version, `pkg/broker` is a thin, logic-free wrapper that only translates MCP requests into `MemoryService` calls.

### Acceptance criteria

- [ ] `agent-mem start --session <id>` launches the MCP server.
- [ ] MCP tool calls are successfully routed to the `MemoryService`.
- [ ] The broker contains zero business logic—only request translation.

---

## Phase 3: Fast Retrieval (FTS5 Search)

**User stories**:
- 5. As an agent, I want to search the project memory using natural language...

### What to build

The retrieval engine using SQLite FTS5. This includes the `read_project_memory` MCP tool, which takes a query string, performs a full-text search, and returns relevant log snippets to the agent.

### Acceptance criteria

- [ ] `read_project_memory` tool is available via MCP.
- [ ] The tool returns relevant log entries based on keyword matching via FTS5.
- [ ] Search results are returned as a clean, structured JSON array for the LLM to parse.

---

## Phase 4: Multi-Agent Safety (Advisory Locking)

**User stories**:
- 6. As an agent, I want to check the lock status of a file...

### What to build

A central locking manager within the broker. It will expose tools to `get_file_lock_status`, `acquire_file_lock`, and `release_file_lock`. This manages the "Gentleman's Agreement" between agents working in the same session.

### Acceptance criteria

- [ ] Agents can claim a lock on a file path.
- [ ] If Agent A holds a lock, Agent B's request to acquire it is rejected.
- [ ] Locks can be released, making the file available for other agents.
- [ ] Lock states are persistent across agent restarts (stored in DB).

---

## Phase 5: The "Shim" & Privacy

**User stories**:
- 3. As a developer, I want to use custom scripts to interact with the broker via a REST API...
- 8. As a developer, I want the broker to respect .mcpignore files...

### What to build

The lightweight REST API and the privacy guardrails. The REST API exposes the same tools as MCP over HTTP. The privacy engine ensures that files matching `.mcpignore` or `.gitignore` patterns are never ingested or logged.

### Acceptance criteria

- [ ] A local HTTP server responds to POST/GET requests for log/search/lock operations.
- [ ] The broker rejects operations involving paths/patterns defined in `.mcpignore`.
- [ ] API keys and secrets (detected by common patterns) are blocked from being appended to logs.

---

## Phase 6: Orchestration & Polishing

**User stories**:
- 9. As a developer, I want to run agents in different repositories... and share the same session...

### What to build

The final CLI orchestration layer and cross-repository verification. This includes `list`, `stop`, and `prune` commands, as well as final integration testing of the "Global Session" concept.

### Acceptance criteria

- [ ] `agent-mem list` shows active and archived sessions.
- [ ] `agent-mem prune` cleans up old sessions and logs.
- [ ] Verified: Two agents in different directories using the same Session ID see each other's updates in real-time.
