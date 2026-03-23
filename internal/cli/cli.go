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
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/peterh/liner"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// ConfigFile represents the rf.yml configuration file structure
type ConfigFile struct {
	Host     string `yaml:"host"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	APIKey   string `yaml:"api_key"`
}

// ConnectionArgs holds the parsed command line arguments
type ConnectionArgs struct {
	Host     string
	Port     int
	Password string
	Key      string
	Type     string
	Username string
	Command  string
	ShowHelp bool
}

// LoadDefaultConfigFile reads the rf.yml file from current directory if it exists
func LoadDefaultConfigFile() (*ConfigFile, error) {
	// Try to read rf.yml from current directory
	data, err := os.ReadFile("rf.yml")
	if err != nil {
		// File doesn't exist, return nil without error
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var config ConfigFile
	if err = yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse rf.yml: %v", err)
	}

	return &config, nil
}

// LoadConfigFileFromPath reads a config file from the specified path
func LoadConfigFileFromPath(path string) (*ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %v", path, err)
	}

	var config ConfigFile
	if err = yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %v", path, err)
	}

	return &config, nil
}

// parseHostPort parses a host:port string and returns host and port
func parseHostPort(hostPort string) (string, int, error) {
	if hostPort == "" {
		return "", -1, nil
	}

	// Split host and port
	parts := strings.Split(hostPort, ":")
	if len(parts) != 2 {
		return "", -1, fmt.Errorf("invalid host format, expected host:port, got: %s", hostPort)
	}

	host := parts[0]
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", -1, fmt.Errorf("invalid port number: %s", parts[1])
	}

	return host, port, nil
}

// ParseConnectionArgs parses command line arguments similar to Python's parse_connection_args
func ParseConnectionArgs(args []string) (*ConnectionArgs, error) {
	// First, scan args to check for help and config file
	var configFilePath string
	var hasOtherFlags bool

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--help" {
			return &ConnectionArgs{ShowHelp: true}, nil
		} else if arg == "-f" && i+1 < len(args) {
			configFilePath = args[i+1]
			i++
		} else if strings.HasPrefix(arg, "-") && arg != "-f" {
			// Check if it's a flag (not a command)
			if arg == "-h" || arg == "-p" || arg == "-w" || arg == "-k" ||
				arg == "-u" || arg == "-admin" || arg == "-user" {
				hasOtherFlags = true
			}
		}
	}

	// Load config file with priority: -f > rf.yml > none
	var config *ConfigFile
	var err error

	if configFilePath != "" {
		// User specified config file via -f
		config, err = LoadConfigFileFromPath(configFilePath)
		if err != nil {
			return nil, err
		}
	} else {
		// Try default rf.yml
		config, err = LoadDefaultConfigFile()
		if err != nil {
			return nil, err
		}
	}

	if config != nil {
		if hasOtherFlags {
			return nil, fmt.Errorf("cannot use command line flags (-h, -p, -w, -k, -u, -admin, -user) when using config file. Please use config file or command line flags, not both")
		}

		return buildArgsFromConfig(config, args)
	}
	// Create a new flag set
	fs := flag.NewFlagSet("ragflow_cli", flag.ContinueOnError)

	// Define flags
	host := fs.String("h", "127.0.0.1", "Admin or RAGFlow service host")
	port := fs.Int("p", -1, "Admin or RAGFlow service port (default: 9381 for admin, 9380 for user)")
	password := fs.String("w", "", "Superuser password")
	key := fs.String("k", "", "API key for authentication")
	_ = fs.String("f", "", "Path to config file (YAML format)") // Already parsed above
	_ = fs.Bool("admin", false, "Run in admin mode (default)")
	userMode := fs.Bool("user", false, "Run in user mode")
	username := fs.String("u", "", "Username (email). In admin mode defaults to admin@ragflow.io, in user mode required")

	// Parse the arguments
	if err = fs.Parse(args); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %v", err)
	}

	// Otherwise, use command line flags
	return buildArgsFromFlags(host, port, password, key, userMode, username, fs.Args())
}

// buildArgsFromConfig builds ConnectionArgs from config file
func buildArgsFromConfig(config *ConfigFile, remainingArgs []string) (*ConnectionArgs, error) {
	result := &ConnectionArgs{}

	// Parse host:port from config file
	if config.Host != "" {
		host, port, err := parseHostPort(config.Host)
		if err != nil {
			return nil, fmt.Errorf("invalid host in config file: %v", err)
		}
		result.Host = host
		result.Port = port
	} else {
		result.Host = "127.0.0.1"
	}

	// Apply auth info from config
	result.Username = config.User
	result.Password = config.Password
	result.Key = config.APIKey

	// Determine mode: if config has auth info, use user mode
	if config.User != "" || config.APIKey != "" {
		result.Type = "user"
	} else {
		result.Type = "admin"
		result.Username = "admin@ragflow.io"
	}

	// Set default port if not specified in config
	if result.Port == -1 {
		if result.Type == "admin" {
			result.Port = 9381
		} else {
			result.Port = 9380
		}
	}

	// Get command from remaining args (no need for quotes or semicolon)
	if len(remainingArgs) > 0 {
		result.Command = strings.Join(remainingArgs, " ") + ";"
	}

	return result, nil
}

// buildArgsFromFlags builds ConnectionArgs from command line flags
func buildArgsFromFlags(host *string, port *int, password *string, key *string, userMode *bool, username *string, remainingArgs []string) (*ConnectionArgs, error) {
	result := &ConnectionArgs{
		Host:     *host,
		Port:     *port,
		Password: *password,
		Key:      *key,
		Username: *username,
	}

	// Determine mode
	if *userMode {
		result.Type = "user"
	} else {
		result.Type = "admin"
	}

	// Set default port based on type if not specified
	if result.Port == -1 {
		if result.Type == "admin" {
			result.Port = 9383
		} else {
			result.Port = 9384
		}
	}

	// Determine username based on mode
	if result.Type == "admin" && result.Username == "" {
		result.Username = "admin@ragflow.io"
	}

	// Get command from remaining args (no need for quotes or semicolon)
	if len(remainingArgs) > 0 {
		result.Command = strings.Join(remainingArgs, " ") + ";"
	}

	return result, nil
}

// PrintUsage prints the CLI usage information
func PrintUsage() {
	fmt.Println(`RAGFlow CLI Client

Usage: ragflow_cli [options] [command]

Options:
  -h string    Admin or RAGFlow service host (default "127.0.0.1")
  -p int       Admin or RAGFlow service port (default 9381 for admin, 9380 for user)
  -w string    Superuser password
  -k string    API key for authentication
  -f string    Path to config file (YAML format)
  -admin       Run in admin mode (default)
  -user        Run in user mode
  -u string    Username (email). In admin mode defaults to admin@ragflow.io
  --help        Show this help message

Configuration File:
  The CLI will automatically read rf.yml from the current directory if it exists.
  Use -f to specify a custom config file path.

  Config file format:
    host: 127.0.0.1:9380
    user: your-email@example.com
    password: your-password
    api_key: your-api-key

  When using a config file, you cannot use other command line flags except -help.
  The command line is only for the SQL command.

Commands:
  SQL commands like: LOGIN USER 'email'; LIST USERS; etc.
`)
}

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
	args    *ConnectionArgs
}

// NewCLI creates a new CLI instance
func NewCLI() (*CLI, error) {
	return NewCLIWithArgs(nil)
}

// NewCLIWithArgs creates a new CLI instance with connection arguments
func NewCLIWithArgs(args *ConnectionArgs) (*CLI, error) {
	// Create liner first
	line := liner.NewLiner()

	// Determine server type
	serverType := "user"
	if args != nil && args.Type != "" {
		serverType = args.Type
	}

	// Create client with password prompt using liner
	client := NewRAGFlowClient(serverType)
	client.PasswordPrompt = line.PasswordPrompt

	// Apply connection arguments if provided
	client.HTTPClient.Host = args.Host
	client.HTTPClient.Port = args.Port

	// Set prompt based on server type
	prompt := "RAGFlow> "
	if serverType == "admin" {
		prompt = "RAGFlow(admin)> "
	}

	return &CLI{
		prompt: prompt,
		client: client,
		line:   line,
		args:   args,
	}, nil
}

// Run starts the interactive CLI
func (c *CLI) Run() error {
	if c.args.Type == "admin" {
		// Allow 3 attempts for password verification
		maxAttempts := 3
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			var input string
			var err error

			// Check if terminal supports password masking
			if term.IsTerminal(int(os.Stdin.Fd())) {
				input, err = c.line.PasswordPrompt("Please input your password: ")
			} else {
				// Terminal doesn't support password masking, use regular prompt
				fmt.Println("Warning: This terminal does not support secure password input")
				input, err = c.line.Prompt("Please input your password (will be visible): ")
			}
			if err != nil {
				fmt.Printf("Error reading input: %v\n", err)
				return err
			}

			input = strings.TrimSpace(input)

			if input == "" {
				if attempt < maxAttempts {
					fmt.Println("Password cannot be empty, please try again")
					continue
				}
				return errors.New("no password provided after 3 attempts")
			}

			// Set the password for verification
			c.args.Password = input

			if err = c.VerifyAuth(); err != nil {
				if attempt < maxAttempts {
					fmt.Printf("Authentication failed: %v (%d/%d attempts)\n", err, attempt, maxAttempts)
					continue
				}
				return fmt.Errorf("authentication failed after %d attempts: %v", maxAttempts, err)
			}

			// Authentication successful
			break
		}
	}

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

		if err = c.execute(input); err != nil {
			fmt.Printf("CLI error: %v\n", err)
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
	var result ResponseIf
	result, err = c.client.ExecuteCommand(cmd)
	if result != nil {
		result.PrintStdout()
	}
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
		c.prompt = "RAGFlow(admin)> "
		fmt.Println("Switched to ADMIN mode")
	case "user":
		c.client.ServerType = "user"
		c.prompt = "RAGFlow> "
		fmt.Println("Switched to USER mode")
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

Commands (User Mode):
  LOGIN USER 'email';                                    - Login as user
  REGISTER USER 'name' AS 'nickname' PASSWORD 'pwd';     - Register new user
  SHOW VERSION;                                          - Show version info
  PING;                                                  - Ping server
  LIST DATASETS;                                         - List user datasets
  LIST AGENTS;                                           - List user agents
  LIST CHATS;                                            - List user chats
  LIST MODEL PROVIDERS;                                  - List model providers
  LIST DEFAULT MODELS;                                   - List default models

Commands (Admin Mode):
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

// RunSingleCommand executes a single command and exits
func (c *CLI) RunSingleCommand(command string) error {
	// Execute the command
	if err := c.execute(command); err != nil {
		return err
	}
	return nil
}

// VerifyAuth verifies authentication if needed
func (c *CLI) VerifyAuth() error {
	if c.args == nil {
		return nil
	}

	if c.args.Username == "" {
		return fmt.Errorf("username is required")
	}

	if c.args.Password == "" {
		return fmt.Errorf("password is required")
	}

	// Create login command with username and password
	cmd := NewCommand("login_user")
	cmd.Params["email"] = c.args.Username
	cmd.Params["password"] = c.args.Password
	_, err := c.client.ExecuteCommand(cmd)
	return err
}
