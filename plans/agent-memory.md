# Plan: agent-memory

> Source PRD: ./PRD.md

## Architectural decisions

Durable decisions that apply across all phases:

- **Path Schema**: `~/.config/agent-broker/` for the database and `sessions/<session_id>/` for Markdown logs.
- **Database**: SQLite in WAL mode with FTS5 enabled.
- **Payload Format**: Human-readable Markdown (.md) mirrored from the relational database.
- **Concurrency**: Go routines with internal mutexes for LWW (Last-Write-Wins) and sequential queuing.
- **Interfaces**: MCP (JSON-RPC over stdio) and REST (HTTP/JSON).

---

## Phase 1: Foundations & The First Append

**User stories**: 
- 1. As a developer, I want to initialize a named session...
- 4. As an agent, I want to append a log of my decisions...
- 7. As a developer, I want the memory to be stored as Markdown files...

### What to build

A minimal Go CLI that can initialize a session and a core persistence layer that writes log entries to both SQLite and a mirrored Markdown file. This phase establishes the directory structure in `~/.config/` and validates the end-to-end "Write to DB -> Sync to File" flow.

### Acceptance criteria

- [ ] `agent-mem init <session_name>` creates the necessary directories and SQLite DB.
- [ ] Internal `Store` module can append a log entry.
- [ ] Appended entries appear in the SQLite `memory_logs` table.
- [ ] Appended entries are immediately written to a human-readable `.md` file in the session directory.

---

## Phase 2: The MCP Interface

**User stories**:
- 2. As a developer, I want my agents to connect to the broker via MCP...
- 4. As an agent, I want to append a log of my decisions...

### What to build

The MCP server implementation. It will run as a daemon (over stdio) and expose the `append_session_log` tool. This allows any MCP-compliant agent to write to the unified memory.

### Acceptance criteria

- [ ] `agent-mem start --session <id>` launches an MCP server on stdio.
- [ ] MCP clients can list the `append_session_log` tool.
- [ ] Calling the tool via MCP successfully persists data to the Phase 1 storage layer.

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
