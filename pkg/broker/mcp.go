package broker

import (
	"context"
	"fmt"
	"time"

	"agent-memory/pkg/decision"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type MCPBroker struct {
	srv     *server.MCPServer
	session *decision.DecisionSession
}

func NewMCPBroker(name, version string, session *decision.DecisionSession) *MCPBroker {
	s := server.NewMCPServer(name, version)
	b := &MCPBroker{
		srv:     s,
		session: session,
	}
	b.registerTools()
	return b
}

func (b *MCPBroker) registerTools() {
	b.srv.AddTool(mcp.NewTool("decide",
		mcp.WithDescription("After completing a task, record what you decided (architecture, code change, plan, or note). Call this after each significant action so other agents can learn from it. Include a diff for code changes."),
		mcp.WithString("agent_id", mcp.Required(), mcp.Description("Your agent ID (e.g., 'backend-agent').")),
		mcp.WithString("type", mcp.Required(), mcp.Description("Decision type: architecture, code_change, plan, preference, note.")),
		mcp.WithString("summary", mcp.Required(), mcp.Description("Brief summary of what was decided.")),
		mcp.WithString("diff", mcp.Description("The actual diff/patch for code changes.")),
	), b.handleDecide)

	b.srv.AddTool(mcp.NewTool("get_context",
		mcp.WithDescription("Call this BEFORE starting any work. Retrieves decisions made by other agents and user preferences in this session. Your own decisions are excluded."),
		mcp.WithString("agent_id", mcp.Required(), mcp.Description("Your agent ID. Your own decisions will be excluded from results.")),
	), b.handleGetContext)

	b.srv.AddTool(mcp.NewTool("prefer",
		mcp.WithDescription("When the user gives a preference or instruction, call this to store it so all agents respect it. No agent_id needed - this is marked as a user preference."),
		mcp.WithString("summary", mcp.Required(), mcp.Description("The user's preference or instruction.")),
	), b.handlePrefer)

	b.srv.AddTool(mcp.NewTool("search_decisions",
		mcp.WithDescription("Search through all decisions by keyword. Optionally filter by decision type (architecture, code_change, plan, preference, note)."),
		mcp.WithString("query", mcp.Required(), mcp.Description("The natural language search query.")),
		mcp.WithString("type", mcp.Description("Optional: filter results to a specific decision type.")),
	), b.handleSearch)

	b.srv.AddTool(mcp.NewTool("get_guide",
		mcp.WithDescription("Returns the full usage guide for agent-memory tools including workflow instructions."),
	), b.handleGetGuide)

	b.srv.AddTool(mcp.NewTool("get_lock_status",
		mcp.WithDescription("Check if a file path is currently locked."),
		mcp.WithString("path", mcp.Required(), mcp.Description("The relative path to the file.")),
	), b.handleLockStatus)

	b.srv.AddTool(mcp.NewTool("acquire_lock",
		mcp.WithDescription("Claim an advisory lock on a file path."),
		mcp.WithString("path", mcp.Required(), mcp.Description("The relative path to the file.")),
	), b.handleAcquireLock)

	b.srv.AddTool(mcp.NewTool("release_lock",
		mcp.WithDescription("Release a previously acquired lock."),
		mcp.WithString("path", mcp.Required(), mcp.Description("The relative path to the file.")),
	), b.handleReleaseLock)

	b.srv.AddTool(mcp.NewTool("compact_session",
		mcp.WithDescription("Archive all decisions to keep search results focused on recent entries."),
	), b.handleCompact)
}

func (b *MCPBroker) handleDecide(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agentID, _ := request.RequireString("agent_id")
	decisionType, _ := request.RequireString("type")
	summary, _ := request.RequireString("summary")

	ctx = decision.WithCaller(ctx, agentID)

	opts := []decision.DecideOption{}
	if diff, _ := request.RequireString("diff"); diff != "" {
		opts = append(opts, decision.WithDiff(diff))
	}

	if err := b.session.Decide(ctx, decision.DecisionType(decisionType), summary, opts...); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to store decision: %v", err)), nil
	}

	return mcp.NewToolResultText("Decision stored successfully."), nil
}

func (b *MCPBroker) handleGetContext(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agentID, _ := request.RequireString("agent_id")
	ctx = decision.WithCaller(ctx, agentID)

	decisions, err := b.session.GetContext(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get context: %v", err)), nil
	}

	if len(decisions) == 0 {
		return mcp.NewToolResultText("No decisions from other agents found."), nil
	}

	responseText := "Decisions from other agents and user:\n"
	for _, d := range decisions {
		tag := ""
		if d.Archived {
			tag = " [archived]"
		}
		responseText += fmt.Sprintf("- [%s][%s]%s: %s\n", d.AgentID, d.DecisionType, tag, d.Content.Summary)
	}
	return mcp.NewToolResultText(responseText), nil
}

func (b *MCPBroker) handlePrefer(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	summary, _ := request.RequireString("summary")
	ctx = decision.WithUser(ctx)

	if err := b.session.Prefer(ctx, summary); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to store preference: %v", err)), nil
	}

	return mcp.NewToolResultText("Preference stored successfully."), nil
}

func (b *MCPBroker) handleSearch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, _ := request.RequireString("query")

	filters := decision.SearchFilters{Limit: 20}
	if dt, _ := request.RequireString("type"); dt != "" {
		filters = filters.WithTypes(decision.DecisionType(dt))
	}

	decisions, err := b.session.Search(ctx, query, filters)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Search failed: %v", err)), nil
	}

	if len(decisions) == 0 {
		return mcp.NewToolResultText("No matching decisions found."), nil
	}

	responseText := "Matching Decisions:\n"
	for _, d := range decisions {
		responseText += fmt.Sprintf("- [%s][%s]: %s\n", d.AgentID, d.DecisionType, d.Content.Summary)
	}
	return mcp.NewToolResultText(responseText), nil
}

func (b *MCPBroker) handleGetGuide(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	guide := `# agent-memory Usage Guide

## Before Starting Work
Always call get_context(agent_id="<your-agent-id>") to learn what other agents have decided and what the user prefers. This avoids duplicate or conflicting work.

## While Working
After each significant action (architecture decision, code change, plan update), call:
  decide(agent_id="<your-agent-id>", type="<type>", summary="<what you did>", diff="<diff if applicable>")

When the user gives a preference or instruction, call:
  prefer(summary="<the user's preference>")

## Searching
Search existing decisions with:
  search_decisions(query="<keywords>", type="<optional filter>")

## File Locking
Before editing a file, check if locked: get_lock_status(path="<file>")
If unlocked, claim it: acquire_lock(path="<file>")
After done editing, release: release_lock(path="<file>")

## Compaction
Call compact_session when the decision log grows too large to archive old entries.`
	return mcp.NewToolResultText(guide), nil
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
	return mcp.NewToolResultText(fmt.Sprintf("File '%s' is UNLOCKED", path)), nil
}

func (b *MCPBroker) handleAcquireLock(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, _ := request.RequireString("path")
	owner := "mcp-agent"
	_, err := b.session.Lock(ctx, path, owner, 1*time.Hour)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Lock acquisition failed: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Lock acquired for: %s (expires in 1 hour)", path)), nil
}

func (b *MCPBroker) handleReleaseLock(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, _ := request.RequireString("path")
	if err := b.session.ReleaseLock(ctx, path); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to release lock: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Lock released for: %s", path)), nil
}

func (b *MCPBroker) handleCompact(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	count, err := b.session.Compact(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Compaction failed: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Archived %d decisions.", count)), nil
}

func (b *MCPBroker) Serve() error {
	return server.ServeStdio(b.srv)
}
