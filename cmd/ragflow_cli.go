package main

import (
	"fmt"
	"os"
	"os/signal"
	"ragflow/internal/common"
	"syscall"

	"ragflow/internal/cli"
)

func main() {
	// Parse command line arguments (skip program name)
	args, err := cli.ParseConnectionArgs(os.Args[1:])
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger with appropriate level
	logLevel := "warn" // Default to warn (quiet mode)
	if args.Verbose {
		logLevel = "info"
	}
	if err := common.Init(logLevel); err != nil {
		fmt.Printf("Warning: Failed to initialize logger: %v\n", err)
	}

	// Show help and exit
	if args.ShowHelp {
		cli.PrintUsage()
		os.Exit(0)
	}

	// Create CLI instance with parsed arguments
	cliApp, err := cli.NewCLIWithArgs(args)
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

	// Check if we have a single command to execute
	if args.Command != nil {
		// Single command mode
		if err = cliApp.RunSingleCommand(args.Command); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Interactive mode
		if err = cliApp.Run(); err != nil {
			fmt.Printf("CLI error: %v\n", err)
			os.Exit(1)
		}
	}
}
