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
		mcp.WithDescription("Store a decision by an agent (architecture, code change, plan, note)."),
		mcp.WithString("agent_id", mcp.Required(), mcp.Description("The agent making this decision.")),
		mcp.WithString("type", mcp.Required(), mcp.Description("Decision type: architecture, code_change, plan, preference, note.")),
		mcp.WithString("summary", mcp.Required(), mcp.Description("Brief summary of the decision.")),
		mcp.WithString("diff", mcp.Description("Optional diff for code changes.")),
	), b.handleDecide)

	b.srv.AddTool(mcp.NewTool("get_context",
		mcp.WithDescription("Retrieve decisions from other agents for cross-agent awareness."),
		mcp.WithString("agent_id", mcp.Required(), mcp.Description("Your agent ID. Your own decisions are excluded.")),
	), b.handleGetContext)

	b.srv.AddTool(mcp.NewTool("prefer",
		mcp.WithDescription("Store a user preference or instruction."),
		mcp.WithString("summary", mcp.Required(), mcp.Description("The user preference or instruction.")),
	), b.handlePrefer)

	b.srv.AddTool(mcp.NewTool("search_decisions",
		mcp.WithDescription("Search decisions using natural language keywords."),
		mcp.WithString("query", mcp.Required(), mcp.Description("The search query.")),
		mcp.WithString("type", mcp.Description("Filter by decision type.")),
	), b.handleSearch)

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
