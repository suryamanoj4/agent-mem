package broker

import (
	"context"
	"fmt"
	"time"

	"agent-memory/pkg/api"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type MCPBroker struct {
	srv     *server.MCPServer
	session api.Session
}

func NewMCPBroker(name, version string, session api.Session) *MCPBroker {
	s := server.NewMCPServer(name, version)
	b := &MCPBroker{
		srv:     s,
		session: session,
	}
	b.registerTools()
	return b
}

func (b *MCPBroker) registerTools() {
	appendTool := mcp.NewTool("append_log",
		mcp.WithDescription("Append a new decision, architectural note, or project state update to the shared memory."),
		mcp.WithString("role", mcp.Required(), mcp.Description("The role of the agent making the update (e.g., 'frontend-expert').")),
		mcp.WithString("content", mcp.Required(), mcp.Description("The actual content/decision to store.")),
	)

	searchTool := mcp.NewTool("search_memory",
		mcp.WithDescription("Search the project's shared memory logs using natural language keywords."),
		mcp.WithString("query", mcp.Required(), mcp.Description("The natural language search query.")),
	)

	lockStatusTool := mcp.NewTool("get_lock_status",
		mcp.WithDescription("Check if a specific file path is currently locked by another agent."),
		mcp.WithString("path", mcp.Required(), mcp.Description("The relative path to the file.")),
	)

	acquireLockTool := mcp.NewTool("acquire_lock",
		mcp.WithDescription("Claim an advisory lock on a file path to signal to other agents that you are working on it."),
		mcp.WithString("path", mcp.Required(), mcp.Description("The relative path to the file.")),
	)

	releaseLockTool := mcp.NewTool("release_lock",
		mcp.WithDescription("Release a previously acquired lock on a file path."),
		mcp.WithString("path", mcp.Required(), mcp.Description("The relative path to the file.")),
	)

	compactTool := mcp.NewTool("compact_session",
		mcp.WithDescription("Archive all non-archived entries in the session to keep search results focused on recent decisions."),
	)

	b.srv.AddTool(appendTool, b.handleAppend)
	b.srv.AddTool(searchTool, b.handleSearch)
	b.srv.AddTool(lockStatusTool, b.handleLockStatus)
	b.srv.AddTool(acquireLockTool, b.handleAcquireLock)
	b.srv.AddTool(releaseLockTool, b.handleReleaseLock)
	b.srv.AddTool(compactTool, b.handleCompact)
}

func (b *MCPBroker) handleAppend(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	role, _ := request.RequireString("role")
	content, _ := request.RequireString("content")

	if err := b.session.Log(ctx, role, content); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to append log: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully appended log for role: %s", role)), nil
}

func (b *MCPBroker) handleSearch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, _ := request.RequireString("query")

	entries, err := b.session.Ask(ctx, query)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Search failed: %v", err)), nil
	}

	if len(entries) == 0 {
		return mcp.NewToolResultText("No matching memory entries found."), nil
	}

	responseText := "Relevant Memory Entries:\n"
	for _, e := range entries {
		responseText += fmt.Sprintf("- [%s]: %s\n", e.Role, e.Content)
	}

	return mcp.NewToolResultText(responseText), nil
}

func (b *MCPBroker) handleLockStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, _ := request.RequireString("path")

	locked, owner, err := b.session.GetLockStatus(ctx, path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get lock status: %v", err)), nil
	}

	if locked {
		return mcp.NewToolResultText(fmt.Sprintf("File '%s' is LOCKED by %s", path, owner)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("File '%s' is currently UNLOCKED", path)), nil
}

func (b *MCPBroker) handleAcquireLock(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, _ := request.RequireString("path")

	// Use a 1-hour TTL. Locks auto-release if the agent crashes.
	owner := "mcp-agent"
	_, err := b.session.Lock(ctx, path, owner, 1*time.Hour)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Lock acquisition failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully acquired lock for: %s (expires in 1 hour)", path)), nil
}

func (b *MCPBroker) handleReleaseLock(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, _ := request.RequireString("path")

	if err := b.session.ReleaseLock(ctx, path); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to release lock: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully released lock for: %s", path)), nil
}

func (b *MCPBroker) handleCompact(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	count, err := b.session.Compact(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Compaction failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Archived %d entries. Search results will now focus on recent entries.", count)), nil
}

func (b *MCPBroker) Serve() error {
	return server.ServeStdio(b.srv)
}
