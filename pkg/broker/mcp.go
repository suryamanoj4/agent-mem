package broker

import (
	"context"
	"fmt"

	"agent-memory/pkg/api"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MCPBroker wraps the MemoryService as an MCP server.
type MCPBroker struct {
	srv     *server.MCPServer
	session api.Session
}

// NewMCPBroker initializes the MCP server and registers tools.
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
	// 1. Tool: append_log
	appendTool := mcp.NewTool("append_log",
		mcp.WithDescription("Append a new decision, architectural note, or project state update to the shared memory."),
		mcp.WithString("role", mcp.Required(), mcp.Description("The role of the agent making the update (e.g., 'frontend-expert').")),
		mcp.WithString("content", mcp.Required(), mcp.Description("The actual content/decision to store.")),
	)

	// 2. Tool: search_memory
	searchTool := mcp.NewTool("search_memory",
		mcp.WithDescription("Search the project's shared memory logs using natural language keywords."),
		mcp.WithString("query", mcp.Required(), mcp.Description("The natural language search query.")),
	)

	// 3. Tool: get_lock_status
	lockStatusTool := mcp.NewTool("get_lock_status",
		mcp.WithDescription("Check if a specific file path is currently locked by another agent."),
		mcp.WithString("path", mcp.Required(), mcp.Description("The relative path to the file.")),
	)

	// 4. Tool: acquire_lock
	acquireLockTool := mcp.NewTool("acquire_lock",
		mcp.WithDescription("Claim an advisory lock on a file path to signal to other agents that you are working on it."),
		mcp.WithString("path", mcp.Required(), mcp.Description("The relative path to the file.")),
	)

	// Register handlers
	b.srv.AddTool(appendTool, b.handleAppend)
	b.srv.AddTool(searchTool, b.handleSearch)
	b.srv.AddTool(lockStatusTool, b.handleLockStatus)
	b.srv.AddTool(acquireLockTool, b.handleAcquireLock)
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

	// Note: In Phase 1, Lock() returns an unlock function.
	// For MCP, we'll store the unlock function in a map or just let it expire with the session.
	// For now, we'll just try to acquire the lock.
	_, err := b.session.Lock(ctx, path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Lock acquisition failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully acquired lock for: %s", path)), nil
}

// Serve starts the MCP server on stdio.
func (b *MCPBroker) Serve() error {
	return server.ServeStdio(b.srv)
}
