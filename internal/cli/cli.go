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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/peterh/liner"
	"gopkg.in/yaml.v3"

	"ragflow/internal/cli/contextengine"
)

// ConfigFile represents the rf.yml configuration file structure
type ConfigFile struct {
	Host     string `yaml:"host"`
	APIToken string `yaml:"api_token"`
	UserName string `yaml:"user_name"`
	Password string `yaml:"password"`
}

// OutputFormat represents the output format type
type OutputFormat string

const (
	OutputFormatTable OutputFormat = "table" // Table format with borders
	OutputFormatPlain OutputFormat = "plain" // Plain text, space-separated (no borders)
	OutputFormatJSON  OutputFormat = "json"  // JSON format (reserved for future use)
)

// ConnectionArgs holds the parsed command line arguments
type ConnectionArgs struct {
	Host         string
	Port         int
	Password     string
	APIToken     string
	UserName     string
	Command      *string  // Original command string (for SQL mode)
	CommandArgs  []string // Split command arguments (for ContextEngine mode)
	IsSQLMode    bool     // true=SQL mode (quoted), false=ContextEngine mode (unquoted)
	ShowHelp     bool
	AdminMode    bool
	OutputFormat OutputFormat // Output format: table, plain, json
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
	// First, scan args to check for help, config file, and admin mode
	var configFilePath string
	var adminMode bool = false
	foundCommand := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		// If we found a command (non-flag arg), stop processing global flags
		// This allows subcommands like "search --help" to handle their own help
		if !strings.HasPrefix(arg, "-") {
			foundCommand = true
			continue
		}
		// Only process --help as global help if it's before any command
		if !foundCommand && (arg == "--help" || arg == "-help") {
			return &ConnectionArgs{ShowHelp: true}, nil
		} else if (arg == "-f" || arg == "--config") && i+1 < len(args) {
			configFilePath = args[i+1]
			i++
		} else if (arg == "-o" || arg == "--output") && i+1 < len(args) {
			// -o/--output is allowed with config file, skip it and its value
			i++
			continue
		} else if arg == "--admin" {
			adminMode = true
		}
	}

	// Load config file with priority: -f > rf.yml > none
	var config *ConfigFile
	var err error

	// Parse arguments manually to support both short and long forms
	// and to handle priority: command line > config file > defaults

	result := &ConnectionArgs{}

	if !adminMode {
		// Only user mode read config file
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

		// Apply config file values first (lower priority)
		if config != nil {
			// Parse host:port from config file
			if config.Host != "" {
				h, port, err := parseHostPort(config.Host)
				if err != nil {
					return nil, fmt.Errorf("invalid host in config file: %v", err)
				}
				result.Host = h
				result.Port = port
			}
			result.UserName = config.UserName
			result.Password = config.Password
			result.APIToken = config.APIToken
		}
	}

	// Get non-flag arguments (command to execute)
	var nonFlagArgs []string

	// Override with command line flags (higher priority)
	// Handle both short and long forms manually
	// Once we encounter a non-flag argument (command), stop parsing global flags
	// Remaining args belong to the subcommand
	foundCommand = false
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// If we've found the command, collect remaining args as subcommand args
		if foundCommand {
			nonFlagArgs = append(nonFlagArgs, arg)
			continue
		}

		switch arg {
		case "-h", "--host":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				hostVal := args[i+1]
				h, port, err := parseHostPort(hostVal)
				if err != nil {
					return nil, fmt.Errorf("invalid host format: %v", err)
				}
				result.Host = h
				result.Port = port
				i++
			}
		case "-t", "--token":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				result.APIToken = args[i+1]
				i++
			}
		case "-u", "--user":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				result.UserName = args[i+1]
				i++
			}
		case "-p", "--password":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				result.Password = args[i+1]
				i++
			}
		case "-f", "--config":
			// Skip config file path (already parsed)
			if i+1 < len(args) {
				i++
			}
		case "-o", "--output":
			// Parse output format
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				format := args[i+1]
				switch format {
				case "plain":
					result.OutputFormat = OutputFormatPlain
				case "json":
					result.OutputFormat = OutputFormatJSON
				default:
					result.OutputFormat = OutputFormatTable
				}
				i++
			}
		case "--admin", "-admin":
			result.AdminMode = true
		case "--help", "-help":
			// Already handled above
			continue
		default:
			// Non-flag argument (command)
			if !strings.HasPrefix(arg, "-") {
				nonFlagArgs = append(nonFlagArgs, arg)
				foundCommand = true
			}
		}
	}

	// Set defaults if not provided
	if result.Host == "" {
		result.Host = "127.0.0.1"
	}
	if result.Port == -1 || result.Port == 0 {
		if result.AdminMode {
			result.Port = 9383
		} else {
			result.Port = 9384
		}
	}

	if result.UserName == "" && result.Password != "" {
		return nil, fmt.Errorf("username (-u/--user) is required when using password (-p/--password)")
	}

	if result.AdminMode {
		result.APIToken = ""
		if result.UserName == "" {
			result.UserName = "admin@ragflow.io"
			result.Password = ""
		}
	} else {
		// For user mode
		// Validate mutual exclusivity: -t and (-u, -p) are mutually exclusive
		hasToken := result.APIToken != ""
		hasUserPass := result.UserName != "" || result.Password != ""

		if hasToken && hasUserPass {
			return nil, fmt.Errorf("cannot use both API token (-t/--token) and username/password (-u/--user, -p/--password). Please use one authentication method")
		}
	}

	// Get command from remaining args (non-flag arguments)
	// Get command from remaining args (non-flag arguments)
	if len(nonFlagArgs) > 0 {
		command := strings.Join(nonFlagArgs, " ")
		result.Command = &command
		fmt.Printf("COMMAND: %s\n", command)
	}

	return result, nil
}

// looksLikeSQL checks if a string looks like a SQL command
func looksLikeSQL(s string) bool {
	s = strings.ToUpper(strings.TrimSpace(s))
	sqlPrefixes := []string{
		"LIST ", "SHOW ", "CREATE ", "DROP ", "ALTER ",
		"LOGIN ", "REGISTER ", "PING", "GRANT ", "REVOKE ",
		"SET ", "UNSET ", "UPDATE ", "DELETE ", "INSERT ",
		"SELECT ", "DESCRIBE ", "EXPLAIN ", "ADD ", "ENABLE ", "DISABLE ", "CHAT ", "USE", "THINK",
	}
	for _, prefix := range sqlPrefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// PrintUsage prints the CLI usage information
func PrintUsage() {
	fmt.Println(`RAGFlow CLI Client

Usage: ragflow_cli [options] [command]

Options:
  -h, --host string      RAGFlow service address (host:port, default "127.0.0.1:9380")
  -t, --token string     API token for authentication
  -u, --user string      Username for authentication
  -p, --password string  Password for authentication
  -f, --config string    Path to config file (YAML format)
  -o, --output string    Output format: table, plain, json (search defaults to json)
  --admin, -admin        Run in admin mode
  --help                 Show this help message

Mode:
  --admin, -admin        Run in admin mode (prompt: RAGFlow(admin)>)
  Default is user mode (prompt: RAGFlow(user)>).

Authentication:
  You can authenticate using either:
    1. API token: -t or --token
    2. Username and password: -u/--user and -p/--password
  Note: These two methods are mutually exclusive.

Configuration File:
  The CLI will automatically read rf.yml from the current directory if it exists.
  Use -f or --config to specify a custom config file path.
  Command line options override config file values.

  Config file format:
    host: 127.0.0.1:9380
    api_token: your-api-token
    user_name: your-username
    password: your-password

  Note: api_token and user_name/password are mutually exclusive in config file.

Commands:
  SQL commands (use quotes): "LIST USERS", "CREATE USER 'email' 'password'", etc.
  Context Engine commands (no quotes): ls datasets, search "keyword", cat path, etc.
  If no command is provided, CLI runs in interactive mode.`)
}

// HistoryFile returns the path to the history file
func HistoryFile() string {
	return os.Getenv("HOME") + "/" + historyFileName
}

const historyFileName = ".ragflow_cli_history"

// CLI represents the command line interface
type CLI struct {
	client        *RAGFlowClient
	contextEngine *contextengine.Engine
	prompt        string
	running       bool
	line          *liner.State
	args          *ConnectionArgs
	outputFormat  OutputFormat // Output format
}

// NewCLI creates a new CLI instance
func NewCLI() (*CLI, error) {
	return NewCLIWithArgs(nil)
}

// NewCLIWithArgs creates a new CLI instance with connection arguments
func NewCLIWithArgs(args *ConnectionArgs) (*CLI, error) {
	// Create liner first
	line := liner.NewLiner()

	// Determine server type based on --admin or --user flag
	// Default to "user" mode if not specified
	serverType := "user"
	if args != nil && args.AdminMode {
		serverType = "admin"
	}

	// Create client with password prompt using liner
	client := NewRAGFlowClient(serverType)
	client.PasswordPrompt = line.PasswordPrompt

	// Apply connection arguments if provided
	if args != nil {
		client.HTTPClient.Host = args.Host
		if args.Port > 0 {
			client.HTTPClient.Port = args.Port
		}

		if args.APIToken != "" {
			client.HTTPClient.APIToken = args.APIToken
		}
	}

	// Apply API token if provided (from config file)
	if args.APIToken != "" {
		client.HTTPClient.APIToken = args.APIToken
		client.HTTPClient.useAPIToken = true
	}

	// Set output format
	client.OutputFormat = args.OutputFormat

	// Auto-login if user and password are provided (from config file)
	if args.UserName != "" && args.Password != "" && args.APIToken == "" {
		if err := client.LoginUserInteractive(args.UserName, args.Password); err != nil {
			line.Close()
			return nil, fmt.Errorf("auto-login failed: %w", err)
		}
	}

	// Set prompt based on server type
	prompt := "RAGFlow(user)> "
	if serverType == "admin" {
		prompt = "RAGFlow(admin)> "
	}

	// Create context engine and register providers
	engine := contextengine.NewEngine()
	engine.RegisterProvider(contextengine.NewDatasetProvider(&httpClientAdapter{client: client.HTTPClient}))
	engine.RegisterProvider(contextengine.NewFileProvider(&httpClientAdapter{client: client.HTTPClient}))

	return &CLI{
		prompt:        prompt,
		client:        client,
		contextEngine: engine,
		line:          line,
		args:          args,
		outputFormat:  args.OutputFormat,
	}, nil
}

// Run starts the interactive CLI
func (c *CLI) Run() error {
	// If username is provided without password, prompt for password
	if c.args != nil && c.args.UserName != "" && c.args.Password == "" && c.args.APIToken == "" {
		maxAttempts := 3
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			fmt.Print("Please input your password: ")

			password, err := ReadPassword()

			if password == "" {
				if attempt < maxAttempts {
					fmt.Println("Password cannot be empty, please try again")
					continue
				}
				return errors.New("no password provided after 3 attempts")
			}

			c.args.Password = password

			if err = c.VerifyAuth(); err != nil {
				if attempt < maxAttempts {
					fmt.Printf("Authentication failed: %v (%d/%d attempts)\n", err, attempt, maxAttempts)
					continue
				}
				return fmt.Errorf("authentication failed after %d attempts: %v", maxAttempts, err)
			}

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

		if err = c.executeNew(input); err != nil {
			fmt.Printf("CLI error: %v\n", err)
		}
	}

	return nil
}

func (c *CLI) executeNew(input string) error {
	p := NewParser(input)
	cmd, err := p.Parse(c.args.AdminMode)
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
		result.PrintOut()
	}
	return err
}

func (c *CLI) execute(input string) error {
	// Determine execution mode based on input and args
	input = strings.TrimSpace(input)

	// Handle meta commands (start with \)
	if strings.HasPrefix(input, "\\") {
		p := NewParser(input)
		cmd, err := p.Parse(c.args.AdminMode)
		if err != nil {
			return err
		}
		if cmd != nil && cmd.Type == "meta" {
			return c.handleMetaCommand(cmd)
		}
	}

	// Check if we should use SQL mode or ContextEngine mode
	isSQLMode := false
	if c.args != nil && len(c.args.CommandArgs) > 0 {
		// Non-interactive mode: use pre-determined mode from args
		isSQLMode = c.args.IsSQLMode
	} else {
		// Interactive mode: determine based on input
		isSQLMode = looksLikeSQL(input)
	}

	if isSQLMode {
		// SQL mode: use parser
		p := NewParser(input)
		cmd, err := p.Parse(c.args.AdminMode)
		if err != nil {
			return err
		}
		if cmd == nil {
			return nil
		}
		// Execute SQL command using the client
		var result ResponseIf
		result, err = c.client.ExecuteCommand(cmd)
		if result != nil {
			result.SetOutputFormat(c.outputFormat)
			result.PrintOut()
		}
		return err
	}

	// ContextEngine mode: execute context engine command
	return c.executeContextEngine(input)
}

// executeContextEngine executes a Context Engine command
func (c *CLI) executeContextEngine(input string) error {
	// Parse input into arguments
	var args []string
	if c.args != nil && len(c.args.CommandArgs) > 0 {
		// Non-interactive mode: use pre-parsed args
		args = c.args.CommandArgs
	} else {
		// Interactive mode: parse input
		args = parseContextEngineArgs(input)
	}

	if len(args) == 0 {
		return fmt.Errorf("no command provided")
	}

	// Check if we have a context engine
	if c.contextEngine == nil {
		return fmt.Errorf("context engine not available")
	}

	cmdType := args[0]
	cmdArgs := args[1:]

	// Build context engine command
	var ceCmd *contextengine.Command

	switch cmdType {
	case "ls", "list":
		// Parse list command arguments
		listOpts, err := parseListCommandArgs(cmdArgs)
		if err != nil {
			return err
		}
		if listOpts == nil {
			// Help was printed
			return nil
		}
		ceCmd = &contextengine.Command{
			Type: contextengine.CommandList,
			Path: listOpts.Path,
			Params: map[string]interface{}{
				"limit": listOpts.Limit,
			},
		}
	case "search":
		// Parse search command arguments
		searchOpts, err := parseSearchCommandArgs(cmdArgs)
		if err != nil {
			return err
		}
		if searchOpts == nil {
			// Help was printed
			return nil
		}
		// Determine the path for provider resolution
		// Use first dir if specified, otherwise default to "datasets"
		searchPath := "datasets"
		if len(searchOpts.Dirs) > 0 {
			searchPath = searchOpts.Dirs[0]
		}
		ceCmd = &contextengine.Command{
			Type: contextengine.CommandSearch,
			Path: searchPath,
			Params: map[string]interface{}{
				"query":     searchOpts.Query,
				"top_k":     searchOpts.TopK,
				"threshold": searchOpts.Threshold,
				"dirs":      searchOpts.Dirs,
			},
		}
	case "cat":
		if len(cmdArgs) == 0 {
			return fmt.Errorf("cat requires a path argument")
		}
		// Handle cat command directly since it returns []byte, not *Result
		content, err := c.contextEngine.Cat(context.Background(), cmdArgs[0])
		if err != nil {
			return err
		}
		if content == nil || len(content) == 0 {
			fmt.Println("(empty file)")
		} else if isBinaryContent(content) {
			return fmt.Errorf("cannot display binary file content")
		}

		fmt.Println(string(content))
		return nil
	default:
		return fmt.Errorf("unknown context engine command: %s", cmdType)
	}

	// Execute the command
	result, err := c.contextEngine.Execute(context.Background(), ceCmd)
	if err != nil {
		return err
	}

	// Print result
	// For search command, default to JSON format if not explicitly set to plain/table
	format := c.outputFormat
	if ceCmd.Type == contextengine.CommandSearch && format != OutputFormatPlain && format != OutputFormatTable {
		format = OutputFormatJSON
	}
	// Get limit for list command
	limit := 0
	if ceCmd.Type == contextengine.CommandList {
		if l, ok := ceCmd.Params["limit"].(int); ok {
			limit = l
		}
	}
	c.printContextEngineResult(result, ceCmd.Type, format, limit)
	return nil
}

// parseContextEngineArgs parses Context Engine command arguments
// Supports simple space-separated args and quoted strings
func parseContextEngineArgs(input string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	var quoteChar rune

	for _, ch := range input {
		switch ch {
		case '"', '\'':
			if !inQuote {
				inQuote = true
				quoteChar = ch
				if current.Len() > 0 {
					args = append(args, current.String())
					current.Reset()
				}
			} else if ch == quoteChar {
				inQuote = false
				args = append(args, current.String())
				current.Reset()
			} else {
				current.WriteRune(ch)
			}
		case ' ', '\t':
			if inQuote {
				current.WriteRune(ch)
			} else if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

// printContextEngineResult prints the result of a context engine command
func (c *CLI) printContextEngineResult(result *contextengine.Result, cmdType contextengine.CommandType, format OutputFormat, limit int) {
	if result == nil {
		return
	}

	switch cmdType {
	case contextengine.CommandList:
		if len(result.Nodes) == 0 {
			fmt.Println("(empty)")
			return
		}
		displayCount := len(result.Nodes)
		if limit > 0 && displayCount > limit {
			displayCount = limit
		}
		if format == OutputFormatPlain {
			// Plain format: simple space-separated, no headers
			for i := 0; i < displayCount; i++ {
				node := result.Nodes[i]
				fmt.Printf("%s %s %s %s\n", node.Name, node.Type, node.Path, node.CreatedAt.Format("2006-01-02 15:04"))
			}
		} else {
			// Table format: with headers and aligned columns
			fmt.Printf("%-30s %-12s %-50s %-20s\n", "NAME", "TYPE", "PATH", "CREATED")
			fmt.Println(strings.Repeat("-", 112))
			for i := 0; i < displayCount; i++ {
				node := result.Nodes[i]
				created := node.CreatedAt.Format("2006-01-02 15:04")
				if node.CreatedAt.IsZero() {
					created = "-"
				}
				// Remove leading "/" from path for display
				displayPath := node.Path
				if strings.HasPrefix(displayPath, "/") {
					displayPath = displayPath[1:]
				}
				fmt.Printf("%-30s %-12s %-50s %-20s\n", node.Name, node.Type, displayPath, created)
			}
		}
		if limit > 0 && result.Total > limit {
			fmt.Printf("\n... and %d more (use -n to show more)\n", result.Total-limit)
		}
		fmt.Printf("Total: %d\n", result.Total)
	case contextengine.CommandSearch:
		if len(result.Nodes) == 0 {
			if format == OutputFormatJSON {
				fmt.Println("[]")
			} else {
				fmt.Println("No results found")
			}
			return
		}
		// Build data for output (same fields for all formats: content, path, score)
		type searchResult struct {
			Content string  `json:"content"`
			Path    string  `json:"path"`
			Score   float64 `json:"score,omitempty"`
		}
		results := make([]searchResult, 0, len(result.Nodes))
		for _, node := range result.Nodes {
			content := node.Name
			if content == "" {
				content = "(empty)"
			}
			displayPath := node.Path
			if strings.HasPrefix(displayPath, "/") {
				displayPath = displayPath[1:]
			}
			var score float64
			if s, ok := node.Metadata["similarity"].(float64); ok {
				score = s
			} else if s, ok := node.Metadata["_score"].(float64); ok {
				score = s
			}
			results = append(results, searchResult{
				Content: content,
				Path:    displayPath,
				Score:   score,
			})
		}
		// Output based on format
		if format == OutputFormatJSON {
			jsonData, err := json.MarshalIndent(results, "", "  ")
			if err != nil {
				fmt.Printf("Error marshaling JSON: %v\n", err)
				return
			}
			fmt.Println(string(jsonData))
		} else if format == OutputFormatPlain {
			// Plain format: simple space-separated, no borders
			fmt.Printf("%-70s  %-50s  %-10s\n", "CONTENT", "PATH", "SCORE")
			for i, sr := range results {
				content := strings.Join(strings.Fields(sr.Content), " ")
				if len(content) > 70 {
					content = content[:67] + "..."
				}
				displayPath := sr.Path
				if len(displayPath) > 50 {
					displayPath = displayPath[:47] + "..."
				}
				scoreStr := "-"
				if sr.Score > 0 {
					scoreStr = fmt.Sprintf("%.4f", sr.Score)
				}
				fmt.Printf("%-70s  %-50s  %-10s\n", content, displayPath, scoreStr)
				if i >= 99 {
					fmt.Printf("\n... and %d more results\n", result.Total-i-1)
					break
				}
			}
			fmt.Printf("\nTotal: %d\n", result.Total)
		} else {
			// Table format: with borders
			col1Width, col2Width, col3Width := 70, 50, 10
			sep := "+" + strings.Repeat("-", col1Width+2) + "+" + strings.Repeat("-", col2Width+2) + "+" + strings.Repeat("-", col3Width+2) + "+"
			fmt.Println(sep)
			fmt.Printf("| %-70s | %-50s | %-10s |\n", "CONTENT", "PATH", "SCORE")
			fmt.Println(sep)
			for i, sr := range results {
				content := strings.Join(strings.Fields(sr.Content), " ")
				if len(content) > 70 {
					content = content[:67] + "..."
				}
				displayPath := sr.Path
				if len(displayPath) > 50 {
					displayPath = displayPath[:47] + "..."
				}
				scoreStr := "-"
				if sr.Score > 0 {
					scoreStr = fmt.Sprintf("%.4f", sr.Score)
				}
				fmt.Printf("| %-70s | %-50s | %-10s |\n", content, displayPath, scoreStr)
				if i >= 99 {
					fmt.Printf("\n... and %d more results\n", result.Total-i-1)
					break
				}
			}
			fmt.Println(sep)
			fmt.Printf("Total: %d\n", result.Total)
		}
	case contextengine.CommandCat:
		// Cat output is handled differently - it returns []byte, not *Result
		// This case should not be reached in normal flow since Cat returns []byte directly
		fmt.Println("Content retrieved")
	}
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
		c.prompt = "RAGFlow(user)> "
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
  LIST TOKENS;                                           - List API tokens
  LIST PROVIDERS;                                        - List available LLM providers
  CREATE TOKEN;                                          - Create new API token
  ADD PROVIDER 'name';                                - Create a provider without API key
  ADD PROVIDER 'name' 'api_key';                      - Create a provider with API key
  DROP TOKEN 'token_value';                              - Delete an API token
  DELETE PROVIDER 'name';                                  - Delete a provider
  SET TOKEN 'token_value';                               - Set and validate API token
  SHOW TOKEN;                                            - Show current API token
  SHOW PROVIDER 'name';                                  - Show provider details
  SHOW CURRENT MODEL;                                    - Show current model settings
  UNSET TOKEN;                                           - Remove current API token
  ALTER PROVIDER 'name' NAME 'new_name';                 - Rename a provider
  USE MODEL 'provider/instance/model';                   - Set current model for chat
  CHAT 'message';                                        - Chat using current model
  CHAT 'provider/instance/model' 'message';              - Chat with specified model

Context Engine Commands (no quotes):
  ls [path]                    - List resources
                                 e.g., ls                   - List root (providers and folders)
                                 e.g., ls datasets          - List all datasets
                                 e.g., ls datasets/kb1      - Show dataset info
                                 e.g., ls myfolder          - List files in 'myfolder' (file_manager)
  list [path]                  - Same as ls
  search [options]             - Search resources in datasets
                                 Use 'search -h' for detailed options
  cat <path>                   - Show file content
                                 e.g., cat files/docs/file.txt  - Show file content
                                 Note: cat datasets or cat datasets/kb1 will error

Examples:
  ragflow_cli -f rf.yml "LIST USERS"           # SQL mode (with quotes)
  ragflow_cli -f rf.yml ls datasets            # Context Engine mode (no quotes)
  ragflow_cli -f rf.yml ls files               # List files in root
  ragflow_cli -f rf.yml cat datasets           # Error: datasets is a directory
  ragflow_cli -f rf.yml ls files/myfolder      # List folder contents

For more information, see documentation.
`
	fmt.Println(help)
}

// Cleanup performs cleanup before exit
func (c *CLI) Cleanup() {
	// Close liner to restore terminal settings
	if c.line != nil {
		c.line.Close()
	}
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
func (c *CLI) RunSingleCommand(command *string) error {
	// Ensure cleanup is called on exit to restore terminal settings
	defer c.Cleanup()

	// Execute the command
	if err := c.executeNew(*command); err != nil {
		return err
	}
	return nil
}

// VerifyAuth verifies authentication if needed
func (c *CLI) VerifyAuth() error {
	if c.args == nil {
		return nil
	}

	// If API token is provided, use it for authentication
	if c.args.APIToken != "" {
		// TODO: Implement API token authentication
		return nil
	}

	// Otherwise, use username/password authentication
	if c.args.UserName == "" {
		return fmt.Errorf("username is required")
	}

	if c.args.Password == "" {
		return fmt.Errorf("password is required")
	}

	// Create login command with username and password
	cmd := NewCommand("login_user")
	cmd.Params["email"] = c.args.UserName
	cmd.Params["password"] = c.args.Password
	_, err := c.client.ExecuteCommand(cmd)
	return err
}

// isBinaryContent checks if content is binary (contains null bytes or invalid UTF-8)
func isBinaryContent(content []byte) bool {
	// Check for null bytes (binary file indicator)
	for _, b := range content {
		if b == 0 {
			return true
		}
	}
	// Check valid UTF-8
	return !utf8.Valid(content)
}

// SearchCommandOptions holds parsed search command options
type SearchCommandOptions struct {
	Query     string
	TopK      int
	Threshold float64
	Dirs      []string
}

// ListCommandOptions holds parsed list command options
type ListCommandOptions struct {
	Path  string
	Limit int
}

// parseSearchCommandArgs parses search command arguments
// Format: search [-d dir1] [-d dir2] ... -q query [-k top_k] [-t threshold]
//
//	search -h|--help (shows help)
func parseSearchCommandArgs(args []string) (*SearchCommandOptions, error) {
	opts := &SearchCommandOptions{
		TopK:      10,
		Threshold: 0.2,
		Dirs:      []string{},
	}

	// Check for help flag
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			printSearchHelp()
			return nil, nil
		}
	}

	// Parse arguments
	i := 0
	for i < len(args) {
		arg := args[i]

		switch arg {
		case "-d", "--dir":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for %s flag", arg)
			}
			opts.Dirs = append(opts.Dirs, args[i+1])
			i += 2
		case "-q", "--query":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for %s flag", arg)
			}
			opts.Query = args[i+1]
			i += 2
		case "-k", "--top-k":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for %s flag", arg)
			}
			topK, err := strconv.Atoi(args[i+1])
			if err != nil {
				return nil, fmt.Errorf("invalid top-k value: %s", args[i+1])
			}
			opts.TopK = topK
			i += 2
		case "-t", "--threshold":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for %s flag", arg)
			}
			threshold, err := strconv.ParseFloat(args[i+1], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid threshold value: %s", args[i+1])
			}
			opts.Threshold = threshold
			i += 2
		default:
			// If it doesn't start with -, it might be a positional argument
			if !strings.HasPrefix(arg, "-") {
				// For backwards compatibility: if no -q flag and this is the last arg, treat as query
				if opts.Query == "" && i == len(args)-1 {
					opts.Query = arg
				} else if opts.Query == "" && len(args) > 0 && i < len(args)-1 {
					// Old format: search [path] query
					// Treat first non-flag as path, rest as query
					opts.Dirs = append(opts.Dirs, arg)
					// Join remaining args as query
					remainingArgs := args[i+1:]
					queryParts := []string{}
					for _, part := range remainingArgs {
						if !strings.HasPrefix(part, "-") {
							queryParts = append(queryParts, part)
						}
					}
					opts.Query = strings.Join(queryParts, " ")
					break
				}
			} else {
				return nil, fmt.Errorf("unknown flag: %s", arg)
			}
			i++
		}
	}

	// Validate required parameters
	if opts.Query == "" {
		return nil, fmt.Errorf("query is required (use -q or --query)")
	}

	// If no directories specified, search in all datasets (empty path means all)
	if len(opts.Dirs) == 0 {
		opts.Dirs = []string{"datasets"}
	}

	return opts, nil
}

// printSearchHelp prints help for the search command
func printSearchHelp() {
	help := `Search command usage: search [options]

Search for content in datasets. Currently only supports searching in datasets.

Options:
  -d, --dir <path>       Directory to search in (can be specified multiple times)
                         Currently only supports paths under 'datasets/'
                         Example: -d datasets/kb1 -d datasets/kb2
  -q, --query <query>    Search query (required)
                         Example: -q "machine learning"
  -k, --top-k <number>   Number of top results to return (default: 10)
                         Example: -k 20
  -t, --threshold <num>  Similarity threshold, 0.0-1.0 (default: 0.2)
                         Example: -t 0.5
  -h, --help             Show this help message

Output:
  Default output format is JSON. Use --output plain or --output table for other formats.

Examples:
  search -d datasets/kb1 -q "neural networks"       # Search in kb1 (JSON output)
  search -d datasets/kb1 -q "AI" --output plain     # Search with plain text output
  search -q "data mining"                           # Search all datasets
  search -q "RAG" -k 20 -t 0.5                      # Return 20 results with threshold 0.5
`
	fmt.Println(help)
}

// printListHelp prints help for the list/ls command
func printListHelp() {
	help := `List command usage: ls [path] [options]

List contents of a path in the context filesystem.

Arguments:
  [path]                 Path to list (default: root - shows all providers and folders)
                         Examples: datasets, datasets/kb1, myfolder

Options:
  -n, --limit <number>   Maximum number of items to display (default: 10)
                         Example: -n 20
  -h, --help             Show this help message

Examples:
  ls                          # List root (all providers and file_manager folders)
  ls datasets                 # List all datasets
  ls datasets/kb1             # List files in kb1 dataset (default 10 items)
  ls myfolder                 # List files in file_manager folder 'myfolder'
  ls -n 5                     # List 5 items at root
`
	fmt.Println(help)
}

// parseListCommandArgs parses list/ls command arguments
// Format: ls [path] [-n limit] [-h|--help]
func parseListCommandArgs(args []string) (*ListCommandOptions, error) {
	opts := &ListCommandOptions{
		Path:  "", // Empty path means list root (all providers and file_manager folders)
		Limit: 10,
	}

	// Check for help flag
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			printListHelp()
			return nil, nil
		}
	}

	// Parse arguments
	i := 0
	for i < len(args) {
		arg := args[i]

		switch arg {
		case "-n", "--limit":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for %s flag", arg)
			}
			limit, err := strconv.Atoi(args[i+1])
			if err != nil {
				return nil, fmt.Errorf("invalid limit value: %s", args[i+1])
			}
			opts.Limit = limit
			i += 2
		default:
			// If it doesn't start with -, treat as path
			if !strings.HasPrefix(arg, "-") {
				opts.Path = arg
			} else {
				return nil, fmt.Errorf("unknown flag: %s", arg)
			}
			i++
		}
	}

	return opts, nil
}
