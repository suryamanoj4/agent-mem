package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"agent-memory/pkg/broker"
	"agent-memory/pkg/service"
	"agent-memory/pkg/store"
	"github.com/spf13/cobra"
)

var (
	sessionID string
	baseDir   string
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "agent-mem",
		Short: "agent-memory is a unified context broker for coding agents.",
	}

	home, _ := os.UserHomeDir()
	defaultDir := filepath.Join(home, ".config", "agent-broker")

	rootCmd.PersistentFlags().StringVar(&baseDir, "dir", defaultDir, "base directory for storage")

	var startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the MCP broker for a specific session",
		Run: func(cmd *cobra.Command, args []string) {
			if sessionID == "" {
				log.Fatal("Error: --session ID is required")
			}

			// 1. Initialize Store
			s, err := store.NewStore(baseDir)
			if err != nil {
				log.Fatalf("Failed to initialize store: %v", err)
			}

			// 2. Initialize Service
			svc := service.NewMemoryService(s)
			defer svc.Close()

			// 3. Connect to Session
			ctx := context.Background()
			sess, err := svc.Connect(ctx, sessionID)
			if err != nil {
				log.Fatalf("Failed to connect to session: %v", err)
			}

			// 4. Start MCP Broker
			fmt.Fprintf(os.Stderr, "Starting agent-memory MCP server for session: %s\n", sessionID)
			b := broker.NewMCPBroker("agent-memory", "0.1.0", sess)
			if err := b.Serve(); err != nil {
				log.Fatalf("MCP Server error: %v", err)
			}
		},
	}

	startCmd.Flags().StringVarP(&sessionID, "session", "s", "", "Session ID to load")
	rootCmd.AddCommand(startCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
