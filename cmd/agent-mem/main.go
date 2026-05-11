package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"agent-memory/pkg/api"
	"agent-memory/pkg/broker"
	"agent-memory/pkg/privacy"
	"agent-memory/pkg/service"
	"agent-memory/pkg/store"
	"github.com/spf13/cobra"
)

var (
	sessionID  string
	baseDir    string
	port       int
	mcpIgnore  string
	olderThan  int
)

func main() {
	home, _ := os.UserHomeDir()
	defaultDir := filepath.Join(home, ".config", "agent-broker")

	var rootCmd = &cobra.Command{
		Use:   "agent-mem",
		Short: "agent-memory is a unified context broker for coding agents.",
	}

	rootCmd.PersistentFlags().StringVar(&baseDir, "dir", defaultDir, "base directory for storage")

	var startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the MCP broker for a specific session",
		Run: func(cmd *cobra.Command, args []string) {
			if sessionID == "" {
				log.Fatal("Error: --session ID is required")
			}

			s, err := store.NewStore(baseDir)
			if err != nil {
				log.Fatalf("Failed to initialize store: %v", err)
			}
			defer s.Close()

			var svc api.MemoryService
			filter, fErr := privacy.NewFilterFromFile(mcpIgnore)
			if fErr == nil {
				svc = service.NewMemoryServiceWithFilter(s, filter)
			} else {
				svc = service.NewMemoryService(s)
			}
			defer svc.Close()

			ctx := context.Background()
			sess, err := svc.Connect(ctx, sessionID)
			if err != nil {
				log.Fatalf("Failed to connect to session: %v", err)
			}

			fmt.Fprintf(os.Stderr, "Starting agent-memory MCP server for session: %s\n", sessionID)
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
			s, err := store.NewStore(baseDir)
			if err != nil {
				log.Fatalf("Failed to initialize store: %v", err)
			}
			defer s.Close()

			svc := service.NewMemoryService(s)
			defer svc.Close()

			// Write PID file
			pidFile := filepath.Join(baseDir, "server.pid")
			os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())+"\n"), 0644)
			defer os.Remove(pidFile)

			addr := fmt.Sprintf("localhost:%d", port)
			b := broker.NewRESTBroker(svc)

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
		Short: "Export a session's memory as Markdown",
		Run: func(cmd *cobra.Command, args []string) {
			if sessionID == "" {
				log.Fatal("Error: --session ID is required")
			}
			s, err := store.NewStore(baseDir)
			if err != nil {
				log.Fatalf("Failed to initialize store: %v", err)
			}
			defer s.Close()
			ctx := context.Background()
			if err := s.Export(ctx, sessionID, os.Stdout); err != nil {
				log.Fatalf("Export failed: %v", err)
			}
		},
	}
	exportCmd.Flags().StringVarP(&sessionID, "session", "s", "", "Session ID to export")

	var listCmd = &cobra.Command{
		Use:   "list",
		Short: "List all sessions in the store",
		Run: func(cmd *cobra.Command, args []string) {
			s, err := store.NewStore(baseDir)
			if err != nil {
				log.Fatalf("Failed to initialize store: %v", err)
			}
			defer s.Close()

			sessions, err := s.ListSessions(context.Background())
			if err != nil {
				log.Fatalf("Failed to list sessions: %v", err)
			}

			if len(sessions) == 0 {
				fmt.Println("No sessions found.")
				return
			}

			fmt.Printf("%-30s %10s %10s %s\n", "SESSION ID", "ENTRIES", "ACTIVE", "LAST UPDATED")
			for _, info := range sessions {
				t := time.Unix(info.UpdatedAt, 0).Format("2006-01-02 15:04")
				fmt.Printf("%-30s %10d %10d %s\n", info.ID, info.EntryCount, info.ActiveCount, t)
			}
		},
	}

	var pruneCmd = &cobra.Command{
		Use:   "prune",
		Short: "Delete archived entries older than N days",
		Run: func(cmd *cobra.Command, args []string) {
			s, err := store.NewStore(baseDir)
			if err != nil {
				log.Fatalf("Failed to initialize store: %v", err)
			}
			defer s.Close()

			cutoff := time.Duration(olderThan) * 24 * time.Hour
			count, err := s.PruneEntries(context.Background(), cutoff)
			if err != nil {
				log.Fatalf("Failed to prune entries: %v", err)
			}
			fmt.Printf("Pruned %d archived entries older than %d days.\n", count, olderThan)
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
