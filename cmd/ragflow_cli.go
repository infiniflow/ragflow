package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"ragflow/internal/cli"
)

func main() {
	// Create CLI instance
	cliApp, err := cli.NewCLI()
	if err != nil {
		fmt.Printf("Failed to create CLI: %v\n", err)
		os.Exit(1)
	}

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cliApp.Cleanup()
		os.Exit(0)
	}()

	// Run CLI
	if err := cliApp.Run(); err != nil {
		fmt.Printf("CLI error: %v\n", err)
		os.Exit(1)
	}
}
