package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"agent-memory/pkg/broker"
	"agent-memory/pkg/decision"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
)

var (
	sessionID string
	baseDir   string
	port      int
	mcpIgnore string
	olderThan int
)

func main() {
	home, _ := os.UserHomeDir()
	defaultDir := filepath.Join(home, ".config", "agent-broker")

	var rootCmd = &cobra.Command{
		Use:   "agent-mem",
		Short: "agent-memory is a shared decision store for coding agents.",
	}

	rootCmd.PersistentFlags().StringVar(&baseDir, "dir", defaultDir, "base directory for storage")

	var startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the MCP broker for a specific session",
		Run: func(cmd *cobra.Command, args []string) {
			if sessionID == "" {
				log.Fatal("Error: --session ID is required")
			}

			store, err := decision.NewSQLiteDecisionStore(baseDir)
			if err != nil {
				log.Fatalf("Failed to initialize store: %v", err)
			}
			defer store.Close()

			dbPath := filepath.Join(baseDir, "decisions.db")
			sqlDB, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
			if err != nil {
				log.Fatalf("Failed to open db for lock manager: %v", err)
			}
			defer sqlDB.Close()

			lockMgr := decision.NewSQLiteLockManager(sqlDB)
			sess := decision.NewDecisionSession(sessionID, store, lockMgr)

			fmt.Fprintf(os.Stderr, "Starting agent-memory MCP server for session: %s\n", sessionID)
			fmt.Fprintf(os.Stderr, `Usage:
  Before starting work, call get_context(agent_id="<your-id>") to see other agents' decisions.
  After changes, call decide(agent_id="<your-id>", type="<type>", summary="<what changed>").
  When user gives preferences, call prefer(summary="<preference>").
  See full guide: call get_guide().
`)
			b := broker.NewMCPBroker("agent-memory", "0.1.0", sess)
			if err := b.Serve(); err != nil {
				log.Fatalf("MCP Server error: %v", err)
			}
		},
	}
	startCmd.Flags().StringVarP(&sessionID, "session", "s", "", "Session ID to load")
	startCmd.Flags().StringVar(&mcpIgnore, "mcpignore", ".mcpignore", "Path to .mcpignore file")

	var serveCmd = &cobra.Command{
		Use:   "serve",
		Short: "Start the REST API server for all sessions",
		Run: func(cmd *cobra.Command, args []string) {
			store, err := decision.NewSQLiteDecisionStore(baseDir)
			if err != nil {
				log.Fatalf("Failed to initialize store: %v", err)
			}
			defer store.Close()

			dbPath := filepath.Join(baseDir, "decisions.db")
			sqlDB, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
			if err != nil {
				log.Fatalf("Failed to open db for lock manager: %v", err)
			}
			defer sqlDB.Close()

			lockMgr := decision.NewSQLiteLockManager(sqlDB)

			pidFile := filepath.Join(baseDir, "server.pid")
			os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())+"\n"), 0644)
			defer os.Remove(pidFile)

			addr := fmt.Sprintf("localhost:%d", port)
			b := broker.NewRESTBroker(store, lockMgr)

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			go func() {
				<-ctx.Done()
				log.Println("Shutting down REST API server...")
				b.Shutdown(context.Background())
			}()

			if err := b.Serve(addr); err != nil {
				log.Fatalf("REST server error: %v", err)
			}
		},
	}
	serveCmd.Flags().IntVarP(&port, "port", "p", 4096, "Port to listen on")

	var exportCmd = &cobra.Command{
		Use:   "export",
		Short: "Export a session's decisions as Markdown",
		Run: func(cmd *cobra.Command, args []string) {
			if sessionID == "" {
				log.Fatal("Error: --session ID is required")
			}
			store, err := decision.NewSQLiteDecisionStore(baseDir)
			if err != nil {
				log.Fatalf("Failed to initialize store: %v", err)
			}
			defer store.Close()
			ctx := context.Background()
			if err := store.Export(ctx, sessionID, os.Stdout); err != nil {
				log.Fatalf("Export failed: %v", err)
			}
		},
	}
	exportCmd.Flags().StringVarP(&sessionID, "session", "s", "", "Session ID to export")

	var listCmd = &cobra.Command{
		Use:   "list",
		Short: "List all sessions in the store",
		Run: func(cmd *cobra.Command, args []string) {
			store, err := decision.NewSQLiteDecisionStore(baseDir)
			if err != nil {
				log.Fatalf("Failed to initialize store: %v", err)
			}
			defer store.Close()

			sessions, err := store.ListSessions(context.Background())
			if err != nil {
				log.Fatalf("Failed to list sessions: %v", err)
			}

			if len(sessions) == 0 {
				fmt.Println("No sessions found.")
				return
			}

			fmt.Printf("%-30s %10s %s\n", "SESSION ID", "ACTIVE", "LAST UPDATED")
			for _, s := range sessions {
				t := s.UpdatedAt.Format("2006-01-02 15:04")
				fmt.Printf("%-30s %10d %s\n", s.SessionID, s.TotalDecisions, t)
			}
		},
	}

	var pruneCmd = &cobra.Command{
		Use:   "prune",
		Short: "Delete archived decisions older than N days",
		Run: func(cmd *cobra.Command, args []string) {
			store, err := decision.NewSQLiteDecisionStore(baseDir)
			if err != nil {
				log.Fatalf("Failed to initialize store: %v", err)
			}
			defer store.Close()

			cutoff := time.Duration(olderThan) * 24 * time.Hour
			count, err := store.PruneEntries(context.Background(), cutoff)
			if err != nil {
				log.Fatalf("Failed to prune decisions: %v", err)
			}
			fmt.Printf("Pruned %d archived decisions older than %d days.\n", count, olderThan)
		},
	}
	pruneCmd.Flags().IntVarP(&olderThan, "days", "d", 30, "Delete entries older than N days")

	var stopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop a running agent-mem REST server",
		Run: func(cmd *cobra.Command, args []string) {
			pidFile := filepath.Join(baseDir, "server.pid")
			data, err := os.ReadFile(pidFile)
			if err != nil {
				log.Fatalf("No running server found (no PID file at %s)", pidFile)
			}

			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				log.Fatalf("Invalid PID file: %v", err)
			}

			process, err := os.FindProcess(pid)
			if err != nil {
				log.Fatalf("Could not find process %d: %v", pid, err)
			}

			if err := process.Signal(syscall.SIGTERM); err != nil {
				log.Fatalf("Failed to stop server: %v", err)
			}

			os.Remove(pidFile)
			fmt.Printf("Stopped server (PID %d)\n", pid)
		},
	}

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(pruneCmd)
	rootCmd.AddCommand(stopCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
