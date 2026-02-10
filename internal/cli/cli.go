package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// CLI represents the command line interface
type CLI struct {
	parser  *Parser
	prompt  string
	running bool
}

// NewCLI creates a new CLI instance
func NewCLI() (*CLI, error) {
	return &CLI{
		prompt: "RAGFlow> ",
	}, nil
}

// Run starts the interactive CLI
func (c *CLI) Run() error {
	c.running = true
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("Welcome to RAGFlow CLI")
	fmt.Println("Type \\? for help, \\q to quit")
	fmt.Println()

	for c.running {
		fmt.Print(c.prompt)

		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		input = strings.TrimSpace(input)

		if input == "" {
			continue
		}

		if err := c.execute(input); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}

	return scanner.Err()
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

	// For now, just print the parsed command
	fmt.Printf("Command: %s\n", cmd.Type)
	if len(cmd.Params) > 0 {
		fmt.Println("Parameters:")
		for k, v := range cmd.Params {
			fmt.Printf("  %s: %v\n", k, v)
		}
	}

	return nil
}

func (c *CLI) handleMetaCommand(cmd *Command) error {
	command := cmd.Params["command"].(string)
	
	switch command {
	case "q", "quit", "exit":
		fmt.Println("Goodbye!")
		c.running = false
	case "?", "h", "help":
		c.printHelp()
	case "c", "clear":
		// Clear screen (simple approach)
		fmt.Print("\033[H\033[2J")
	default:
		return fmt.Errorf("unknown meta command: \\%s", command)
	}
	return nil
}

func (c *CLI) printHelp() {
	help := `
RAGFlow CLI Help
================

SQL Commands:
  LOGIN USER 'email';                                    - Login as user
  REGISTER USER 'name' AS 'nickname' PASSWORD 'pwd';     - Register new user
  SHOW VERSION;                                          - Show version info
  SHOW CURRENT USER;                                     - Show current user
  LIST USERS;                                            - List all users
  LIST SERVICES;                                         - List services
  PING;                                                  - Ping server
  ... and many more

Meta Commands:
  \\? or \\h      - Show this help
  \\q or \\quit   - Exit CLI
  \\c or \\clear  - Clear screen

For more information, see documentation.
`
	fmt.Println(help)
}

// Cleanup performs cleanup before exit
func (c *CLI) Cleanup() {
	fmt.Println("\nCleaning up...")
}
