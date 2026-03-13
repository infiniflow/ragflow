//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package cli

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"github.com/peterh/liner"
)

// HistoryFile returns the path to the history file
func HistoryFile() string {
	return os.Getenv("HOME") + "/" + historyFileName
}

const historyFileName = ".ragflow_cli_history"

// CLI represents the command line interface
type CLI struct {
	client  *RAGFlowClient
	prompt  string
	running bool
	line    *liner.State
}

// NewCLI creates a new CLI instance
func NewCLI() (*CLI, error) {
	// Create liner first
	line := liner.NewLiner()

	// Create client with password prompt using liner
	client := NewRAGFlowClient("user") // Default to user mode
	client.PasswordPrompt = line.PasswordPrompt

	return &CLI{
		prompt: "RAGFlow> ",
		client: client,
		line:   line,
	}, nil
}

// Run starts the interactive CLI
func (c *CLI) Run() error {
	c.running = true

	// Load history from file
	histFile := HistoryFile()
	if f, err := os.Open(histFile); err == nil {
		c.line.ReadHistory(f)
		f.Close()
	}

	// Save history on exit
	defer func() {
		if f, err := os.Create(histFile); err == nil {
			c.line.WriteHistory(f)
			f.Close()
		}
		c.line.Close()
	}()

	fmt.Println("Welcome to RAGFlow CLI")
	fmt.Println("Type \\? for help, \\q to quit")
	fmt.Println()

	for c.running {
		input, err := c.line.Prompt(c.prompt)
		if err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		input = strings.TrimSpace(input)

		if input == "" {
			continue
		}

		// Add to history (skip meta commands)
		if !strings.HasPrefix(input, "\\") {
			c.line.AppendHistory(input)
		}

		if err := c.execute(input); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}

	return nil
}

func (c *CLI) execute(input string) error {
	p := NewParser(input)
	cmd, err := p.Parse()
	if err != nil {
		return err
	}

	if cmd == nil {
		return nil
	}

	// Handle meta commands
	if cmd.Type == "meta" {
		return c.handleMetaCommand(cmd)
	}

	// Execute the command using the client
	_, err = c.client.ExecuteCommand(cmd)
	return err
}

func (c *CLI) handleMetaCommand(cmd *Command) error {
	command := cmd.Params["command"].(string)
	args, _ := cmd.Params["args"].([]string)

	switch command {
	case "q", "quit", "exit":
		fmt.Println("Goodbye!")
		c.running = false
	case "?", "h", "help":
		c.printHelp()
	case "c", "clear":
		// Clear screen (simple approach)
		fmt.Print("\033[H\033[2J")
	case "admin":
		c.client.ServerType = "admin"
		c.client.HTTPClient.Port = 9381
		c.prompt = "RAGFlow(admin)> "
		fmt.Println("Switched to ADMIN mode (port 9381)")
	case "user":
		c.client.ServerType = "user"
		c.client.HTTPClient.Port = 9380
		c.prompt = "RAGFlow> "
		fmt.Println("Switched to USER mode (port 9380)")
	case "host":
		if len(args) == 0 {
			fmt.Printf("Current host: %s\n", c.client.HTTPClient.Host)
		} else {
			c.client.HTTPClient.Host = args[0]
			fmt.Printf("Host set to: %s\n", args[0])
		}
	case "port":
		if len(args) == 0 {
			fmt.Printf("Current port: %d\n", c.client.HTTPClient.Port)
		} else {
			port, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid port number: %s", args[0])
			}
			if port < 1 || port > 65535 {
				return fmt.Errorf("port must be between 1 and 65535")
			}
			c.client.HTTPClient.Port = port
			fmt.Printf("Port set to: %d\n", port)
		}
	case "status":
		fmt.Printf("Server: %s:%d (mode: %s)\n", c.client.HTTPClient.Host, c.client.HTTPClient.Port, c.client.ServerType)
	default:
		return fmt.Errorf("unknown meta command: \\%s", command)
	}
	return nil
}

func (c *CLI) printHelp() {
	help := `
RAGFlow CLI Help
================

Meta Commands:
  \admin        - Switch to ADMIN mode (port 9381)
  \user         - Switch to USER mode (port 9380)
  \host [ip]    - Show or set server host (default: 127.0.0.1)
  \port [num]   - Show or set server port (default: 9380 for user, 9381 for admin)
  \status       - Show current connection status
  \? or \h      - Show this help
  \q or \quit   - Exit CLI
  \c or \clear  - Clear screen

SQL Commands (User Mode):
  LOGIN USER 'email';                                    - Login as user
  REGISTER USER 'name' AS 'nickname' PASSWORD 'pwd';     - Register new user
  SHOW VERSION;                                          - Show version info
  PING;                                                  - Ping server
  LIST DATASETS;                                         - List user datasets
  LIST AGENTS;                                           - List user agents
  LIST CHATS;                                            - List user chats
  LIST MODEL PROVIDERS;                                  - List model providers
  LIST DEFAULT MODELS;                                   - List default models

SQL Commands (Admin Mode):
  LOGIN USER 'email';                                    - Login as admin
  LIST USERS;                                            - List all users
  SHOW USER 'email';                                     - Show user details
  CREATE USER 'email' 'password';                        - Create new user
  DROP USER 'email';                                     - Delete user
  ALTER USER PASSWORD 'email' 'new_password';            - Change user password
  ALTER USER ACTIVE 'email' on/off;                      - Activate/deactivate user
  GRANT ADMIN 'email';                                   - Grant admin role
  REVOKE ADMIN 'email';                                  - Revoke admin role
  LIST SERVICES;                                         - List services
  SHOW SERVICE <id>;                                     - Show service details
  PING;                                                  - Ping server
  ... and many more

Meta Commands:
  \? or \h      - Show this help
  \q or \quit   - Exit CLI
  \c or \clear  - Clear screen

For more information, see documentation.
`
	fmt.Println(help)
}

// Cleanup performs cleanup before exit
func (c *CLI) Cleanup() {
	fmt.Println("\nCleaning up...")
}

// RunInteractive runs the CLI in interactive mode
func RunInteractive() error {
	cli, err := NewCLI()
	if err != nil {
		return fmt.Errorf("failed to create CLI: %v", err)
	}

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cli.Cleanup()
		os.Exit(0)
	}()

	return cli.Run()
}
