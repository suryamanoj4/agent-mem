# PRD: agent-memory

## Problem Statement

Developers using multiple coding agents (e.g., a frontend agent, a backend agent, and a documentation agent) face "context fragmentation." Each agent operates in its own isolated memory space, leading to redundant instructions, conflicting architectural decisions, and a lack of a unified project "consciousness." Current solutions are often model-specific or require heavy, non-portable integration code.

## Solution

`agent-memory` is a lightning-fast, highly concurrent memory broker that runs as a local Go daemon. It unifies the context windows of multiple agents into a single persistent, global state. By leveraging the Model Context Protocol (MCP) and SQLite, it provides a cross-repository memory synchronization layer that is air-gapped, human-readable (via Markdown logs), and requires zero model-specific code.

## User Stories

1. As a developer, I want to initialize a named session (e.g., `feature-auth`), so that I can group the work of multiple agents under one context.
2. As a developer, I want my agents to connect to the broker via MCP, so that they can automatically discover tools for reading and writing memory.
3. As a developer, I want to use custom scripts to interact with the broker via a REST API, so that I can integrate non-MCP-native tools.
4. As an agent, I want to append a log of my decisions to the session, so that other agents in the same session can stay informed.
5. As an agent, I want to search the project memory using natural language, so that I can retrieve relevant past decisions without reading the entire log.
6. As an agent, I want to check the lock status of a file before editing it, so that I don't conflict with another agent working on the same file.
7. As a developer, I want the memory to be stored as Markdown files in a global directory, so that I can easily audit the logs using standard text editors.
8. As a developer, I want the broker to respect `.mcpignore` files, so that sensitive information like API keys is never ingested into the memory.
9. As a developer, I want to run agents in different repositories (e.g., `frontend` and `backend`) and have them share the same session, so that they maintain architectural consistency.

## Implementation Decisions

### Architecture
- **Language:** Go for high concurrency and low-latency performance.
- **Daemon:** A single binary acting as an MCP Server (stdio) and a REST Server (localhost).
- **Storage Engine:** SQLite in WAL (Write-Ahead Logging) mode for concurrent read/write operations.
- **Payload Mirroring:** Every memory entry is mirrored from SQLite to physical `.md` files in `~/.config/agent-broker/sessions/<session_id>/`.
- **Search:** SQLite FTS5 extension for blazing-fast full-text search over memory logs.

### Modules
- **`cmd/agent-mem`**: CLI entrypoint for session orchestration (`start`, `init`, `stop`).
- **`pkg/broker`**: Handles MCP JSON-RPC and REST HTTP requests.
- **`pkg/store`**: Manages SQLite, FTS5, and Markdown file synchronization.
- **`pkg/privacy`**: Implements `.mcpignore` logic to filter sensitive paths.

### Coordination
- **Session ID:** Agents are explicitly passed a `session_id` at startup.
- **Advisory Locking:** A `get_file_lock_status` tool provides a "Gentleman's Agreement" for file access, enforced via agent system prompts.
- **Conflict Resolution:** Last-Write-Wins (LWW) for log appends, managed by Go's internal synchronization primitives.

## Testing Decisions

- **Behavioral Testing:** Tests will focus on external behavior (e.g., "If I append a log via REST, can I search for it via MCP?").
- **Concurrency Testing:** The `pkg/store` will be tested under heavy load to ensure SQLite WAL mode handles multiple concurrent agent writes without corruption.
- **Markdown Integrity:** Tests will verify that the filesystem state matches the database state after every write.
- **Privacy Filtering:** Unit tests for the ignore engine to ensure blocked patterns (like `.env`) are never processed.

## Out of Scope

- **Multi-machine Synchronization:** Memory is strictly local to the developer's machine.
- **Built-in Log Viewer:** The CLI will not provide a TUI for reading logs; users should use their preferred Markdown viewers.
- **Automatic Repository Detection:** Sessions must be explicitly initialized and attached by the user.

## Further Notes

- **.mcpignore:** This file will inherit from `.gitignore` by default but allow for specific exclusions unique to the agent's context.
- **Portability:** The use of Go and SQLite ensures the broker can run on Linux, macOS, and Windows with minimal dependencies.
