# agent-memory

**agent-memory** is a lightning-fast, highly concurrent memory broker designed to unify the context windows of multiple coding agents into a single persistent state. 

Built in Go, it runs as a local daemon and provides air-gapped, cross-repository memory synchronization via the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) and a lightweight REST API.

## 🚀 Vision

In a multi-agent workflow, agents often suffer from "context fragmentation"—the frontend agent doesn't know what the backend agent decided, leading to architectural drift. `agent-memory` acts as a central "consciousness" for your project, ensuring every agent operates from the same source of truth.

## 🏗️ Architecture

- **Core Engine**: Written in Go for high concurrency and low latency.
- **Storage**: SQLite in WAL (Write-Ahead Logging) mode for robust, concurrent persistence.
- **Payloads**: Human-readable Markdown (.md) mirrored from the database for easy auditing.
- **Search**: SQLite FTS5 for blazing-fast natural language retrieval.
- **Interfaces**: 
  - **MCP**: Native integration for premier coding agents (Claude, Cursor, etc.).
  - **REST**: Lightweight shim for custom scripts and legacy models.
- **Privacy**: Local-first, air-gapped design with strict `.mcpignore` support.

## 📂 Project Structure

- `cmd/agent-mem`: CLI entrypoint for session management and daemon orchestration.
- `pkg/broker`: MCP and REST server implementations.
- `pkg/store`: Persistence layer (SQLite + Markdown mirroring + FTS5).
- `pkg/privacy`: Privacy guardrails and `.mcpignore` logic.
- `pkg/api`: Shared types and interface contracts.

## 🛠️ Getting Started

### Prerequisites
- Go 1.21+
- SQLite3

### Installation
*(Coming Soon)*

```bash
# Initialize a new session
agent-mem init my-feature-branch

# Start the broker
agent-mem start --session my-feature-branch
```

## 📜 Documentation
- [PRD.md](./PRD.md): Detailed Product Requirements Document.
- [plans/agent-memory.md](./plans/agent-memory.md): Implementation Roadmap.

## 📄 License
MIT
