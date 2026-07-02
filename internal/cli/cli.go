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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	//"os/signal"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/peterh/liner"
	"gopkg.in/yaml.v3"

	"ragflow/internal/cli/filesystem"
)

type APIServerConfig struct {
	Name         string  `yaml:"name"`
	Host         string  `yaml:"host"`
	UserName     *string `yaml:"user_name"`
	UserPassword *string `yaml:"password"`
	APIKey       *string `yaml:"api_key"`
	KeyFile      *string `yaml:"key_file"`
	IP           string
	Port         int
}

// ConfigFile represents the rf.yml configuration file structure
type ConfigFile struct {
	Host         string                      `yaml:"host"`      // default API server host
	APIKey       string                      `yaml:"api_key"`   // default API server api key
	UserName     string                      `yaml:"user_name"` // default API server user name
	Password     string                      `yaml:"password"`  // default API server password
	APIServerMap map[string]*APIServerConfig `yaml:"api_servers"`
}

// OutputFormat represents the output format type
type OutputFormat string

const (
	OutputFormatTable OutputFormat = "table" // Table format with borders
	OutputFormatPlain OutputFormat = "plain" // Plain text, space-separated (no borders)
	OutputFormatJSON  OutputFormat = "json"  // JSON format (reserved for future use)
)

type CommandLineMode string

const (
	APIMode          CommandLineMode = "api"
	AdminMode        CommandLineMode = "admin"
	IngestorMode     CommandLineMode = "ingestor"  // If we want to access ingestor
	CollectorMode    CommandLineMode = "collector" // If we want to access collector
	DefaultAPIServer                 = "default"
)

type CommandLineConfig struct {
	CLIMode           CommandLineMode
	AdminClientConfig *AdminModeConfig
	APIClientConfig   APIModeConfig
	ShowHelp          bool
	Verbose           bool
	Interactive       bool
	OutputFormat      OutputFormat
	Command           *string
}

type AdminModeConfig struct {
	AdminHost     string
	AdminPort     int
	AdminName     *string
	AdminPassword *string
	KeyFile       *string
	//AdminCommand  *string
}

type APIModeConfig struct {
	CurrentAPIServer string
	APIServerMap     map[string]*APIServerConfig
}

func (c *CommandLineConfig) Print() {
	b, err := json.MarshalIndent(c, "", "  ")
	if err == nil {
		fmt.Println(string(b))
	}
}

func ParseArgs(args []string) (*CommandLineConfig, error) {
	commandLineConfig := &CommandLineConfig{
		CLIMode:           APIMode,
		AdminClientConfig: nil,
		ShowHelp:          false,
		Verbose:           false,
		Interactive:       true,
		OutputFormat:      OutputFormatTable,
		Command:           nil,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch arg {
		case "-o", "--output":
			// Parse output format
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				format := args[i+1]
				switch format {
				case "plain":
					commandLineConfig.OutputFormat = OutputFormatPlain
				case "json":
					commandLineConfig.OutputFormat = OutputFormatJSON
				default:
					commandLineConfig.OutputFormat = OutputFormatTable
				}
				i++
			}
		case "-v", "--verbose":
			commandLineConfig.Verbose = true
		case "--admin", "-admin":
			commandLineConfig.CLIMode = AdminMode
		case "--help", "-help":
			commandLineConfig.ShowHelp = true
		default:
			if !strings.HasPrefix(arg, "-") {
				commandLineConfig.Interactive = false
			}
		}
	}

	var commandArgs []string
	var foundCommand bool

	switch commandLineConfig.CLIMode {
	case APIMode:
		defaultApiServerConfig := &APIServerConfig{
			UserName:     nil,
			UserPassword: nil,
			APIKey:       nil,
		}

		configFile := "rf.yml"
		for i := 0; i < len(args); i++ {
			arg := args[i]

			// Handle known global flags (already parsed in first pass).
			// Intercept here regardless of position so they are never
			// mistaken for command args or unknown flags downstream.
			switch arg {
			case "-o", "--output":
				if i+1 < len(args) {
					i++
				}
				continue
			case "-v", "--verbose", "--help", "-help":
				continue
			case "--admin", "-admin":
				return nil, fmt.Errorf("unexpected parameter: --admin")
			}

			// If we've found the command, collect remaining args
			if foundCommand {
				commandArgs = append(commandArgs, arg)
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
					defaultApiServerConfig.IP = h
					defaultApiServerConfig.Port = port
					i++
				}
			case "-t", "--token":
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					defaultApiServerConfig.APIKey = &args[i+1]
					i++
				}
			case "-u", "--user":
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					defaultApiServerConfig.UserName = &args[i+1]
					i++
				}
			case "-p", "--password":
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					defaultApiServerConfig.UserPassword = &args[i+1]
					i++
				}
			case "-f", "--config":
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					configFile = args[i+1]
					// Convert to absolute path immediately
					if !filepath.IsAbs(configFile) {
						absPath, err := filepath.Abs(configFile)
						if err == nil {
							configFile = absPath
						}
					}
					i++
				}
			case "-k", "--key":
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					defaultApiServerConfig.KeyFile = &args[i+1]
					i++
				}
			default:
				// Non-flag argument (command)
				if !strings.HasPrefix(arg, "-") {
					commandArgs = append(commandArgs, arg)
					foundCommand = true
				}
			}
		}

		var config ConfigFile
		data, err := os.ReadFile(configFile)
		if err == nil {
			if err = yaml.Unmarshal(data, &config); err != nil {
				return nil, fmt.Errorf("failed to parse rf.yml: %v", err)
			}
			if config.Host != "" {
				var h string
				var port int
				h, port, err = parseHostPort(config.Host)
				if err != nil {
					return nil, fmt.Errorf("invalid host in config file: %v", err)
				}
				if defaultApiServerConfig.IP == "" {
					defaultApiServerConfig.IP = h
				}
				if defaultApiServerConfig.Port == 0 {
					defaultApiServerConfig.Port = port
				}
			}
			if config.UserName != "" {
				if defaultApiServerConfig.UserName == nil {
					defaultApiServerConfig.UserName = &config.UserName
				}
			}
			if config.Password != "" {
				if defaultApiServerConfig.UserPassword == nil {
					defaultApiServerConfig.UserPassword = &config.Password
				}
			}
			if config.APIKey != "" {
				if defaultApiServerConfig.APIKey == nil {
					defaultApiServerConfig.APIKey = &config.APIKey
				}
			}
		} else {
			if configFile == "rf.yml" && os.IsNotExist(err) {
			} else {
				return nil, fmt.Errorf("failed to read %s: %v", configFile, err)
			}
		}

		if defaultApiServerConfig.IP == "" {
			defaultApiServerConfig.IP = "127.0.0.1"
		}
		if defaultApiServerConfig.Port == 0 {
			defaultApiServerConfig.Port = 9384
		}

		commandLineConfig.APIClientConfig.APIServerMap = config.APIServerMap
		if commandLineConfig.APIClientConfig.APIServerMap == nil {
			commandLineConfig.APIClientConfig.APIServerMap = make(map[string]*APIServerConfig)
		}
		if commandLineConfig.APIClientConfig.APIServerMap[DefaultAPIServer] != nil {
			return nil, fmt.Errorf("'Default' API server config should be in api_servers")
		}
		commandLineConfig.APIClientConfig.APIServerMap[DefaultAPIServer] = defaultApiServerConfig
		commandLineConfig.APIClientConfig.CurrentAPIServer = DefaultAPIServer
	case AdminMode:
		AdminConfig := &AdminModeConfig{
			AdminHost: "127.0.0.1",
			AdminPort: 9383,
			//AdminName:     "admin@ragflow.io",
			//AdminPassword: "admin",
		}

		for i := 0; i < len(args); i++ {
			arg := args[i]

			// Handle known global flags regardless of position
			switch arg {
			case "-o", "--output":
				if i+1 < len(args) {
					i++
				}
				continue
			case "-v", "--verbose", "--admin", "-admin", "--help", "-help":
				continue
			case "-t", "--token":
				return nil, fmt.Errorf("token is invalid in admin mode")
			case "-f", "--config":
				return nil, fmt.Errorf("config is invalid in admin mode")
			}

			// If we've found the command, collect remaining args
			if foundCommand {
				commandArgs = append(commandArgs, arg)
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
					AdminConfig.AdminHost = h
					AdminConfig.AdminPort = port
					i++
				}
			case "-u", "--user":
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					AdminConfig.AdminName = &args[i+1]
					i++
				}
			case "-k", "--key":
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					AdminConfig.KeyFile = &args[i+1]
					i++
				}
			case "-p", "--password":
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					AdminConfig.AdminPassword = &args[i+1]
					i++
				}
			default:
				// Non-flag argument (command)
				if !strings.HasPrefix(arg, "-") {
					commandArgs = append(commandArgs, arg)
					foundCommand = true
				}
			}
		}
		commandLineConfig.AdminClientConfig = AdminConfig
	}

	commandArgsLen := len(commandArgs)
	if commandArgsLen > 0 {
		if commandArgsLen == 1 {
			commandLineConfig.Command = &commandArgs[0]
		} else {
			ApiCommand := strings.Join(commandArgs, " ")
			commandLineConfig.Command = &ApiCommand
		}
	}

	return commandLineConfig, nil
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

// PrintUsage prints the CLI usage information
func PrintUsage() {
	fmt.Println(`RAGFlow CLI Client

Usage: ragflow-cli [options] [command]

Options:
  -h, --host string      RAGFlow service address (host:port, default "127.0.0.1:9380")
  -t, --token string     API token for authentication
  -u, --user string      Username for authentication
  -p, --password string  Password for authentication
  -f, --config string    Path to config file (YAML format)
  -o, --output string    Output format: table, plain, json (search defaults to json)
  -v, --verbose          Enable verbose logging (shows debug info)
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
  Filesystem commands (no quotes): ls datasets, search "keyword", cat path, etc.
  Skill commands:
    install-skill <space> <path|url> [options]  Install a skill from local path or remote URL
    uninstall-skill <space> <skill-name>         Remove an installed skill
    search skills -q <query> [--space space1]   Search skills in a space
  If no command is provided, CLI runs in interactive mode.`)
}

// HistoryFile returns the path to the history file
func HistoryFile() string {
	return os.Getenv("HOME") + "/" + historyFileName
}

const historyFileName = ".ragflow_cli_history"

// CLI represents the command line interface
type CLI struct {
	running bool
	line    *liner.State

	APIServerClientMap map[string]*HTTPClient
	AdminServerClient  *HTTPClient
	PasswordPrompt     PasswordPromptFunc // Function for password input
	ContextEngine      *filesystem.Engine // Context Engine for virtual filesystem
	CurrentModel       *CurrentModel      // Current model configuration
	Config             *CommandLineConfig
}

func NewCLIWithConfig(commandLineConfig *CommandLineConfig) (*CLI, error) {
	// Create liner first
	line := liner.NewLiner()

	cli := &CLI{
		line:   line,
		Config: commandLineConfig,
	}

	if commandLineConfig.CLIMode == APIMode {
		apiServerConfig := commandLineConfig.APIClientConfig.APIServerMap[commandLineConfig.APIClientConfig.CurrentAPIServer]
		httpClient := NewHTTPClient()
		httpClient.Host = apiServerConfig.IP
		httpClient.Port = apiServerConfig.Port
		if apiServerConfig.APIKey != nil {
			httpClient.APIKey = apiServerConfig.APIKey
			httpClient.useAPIKey = true
		}
		cli.APIServerClientMap = map[string]*HTTPClient{
			cli.Config.APIClientConfig.CurrentAPIServer: httpClient,
		}
		// Auto-login if user and password are provided (from config file)
		if apiServerConfig.UserName != nil && apiServerConfig.UserPassword != nil && apiServerConfig.APIKey == nil {
			if err := cli.LoginUserInteractive(*apiServerConfig.UserName, *apiServerConfig.UserPassword); err != nil {
				line.Close()
				return nil, fmt.Errorf("auto-login failed: %w", err)
			}
		}

		engine := filesystem.NewEngine()

		// Register providers
		// TODO: if http config change, engine http config won't be updated. They should share the same config
		engine.RegisterProvider(filesystem.NewDatasetProvider(&httpClientAdapter{httpClient}))
		engine.RegisterProvider(filesystem.NewFileProvider(&httpClientAdapter{httpClient}))
		engine.RegisterProvider(filesystem.NewSkillProvider(&httpClientAdapter{httpClient}))

		cli.ContextEngine = engine
	} else if commandLineConfig.CLIMode == AdminMode {
		httpClient := NewHTTPClient()
		httpClient.Host = commandLineConfig.AdminClientConfig.AdminHost
		httpClient.Port = commandLineConfig.AdminClientConfig.AdminPort
		cli.AdminServerClient = httpClient

		adminServerConfig := commandLineConfig.AdminClientConfig
		// Auto-login if user and password are provided (from config file)
		if adminServerConfig.AdminName != nil && adminServerConfig.AdminPassword != nil {
			if err := cli.LoginUserInteractive(*adminServerConfig.AdminName, *adminServerConfig.AdminPassword); err != nil {
				line.Close()
				return nil, fmt.Errorf("auto-login failed: %w", err)
			}
		}

	} else {
		return nil, fmt.Errorf("invalid CLI mode: %s", commandLineConfig.CLIMode)
	}

	return cli, nil
}

// sanitizeCLIError returns an operator-safe rendering of a CLI
// command error. Many command handlers build their errors via
// fmt.Errorf("... %s ...", userInput) where userInput can be a
// dataset name, file path, or partial command containing secrets;
// printing err.Error() verbatim would echo that back to the
// operator's terminal in cleartext. We keep the error class (e.g.
// "not found", "invalid argument") and drop the interpolated
// user-controlled values. The full error is still available via
// err.Error() for the caller's own logging.
func sanitizeCLIError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// Strip every single-quoted span. Many command handlers interpolate
	// user-controlled values via fmt.Errorf("... '%s' ... '%s' ...", a, b)
	// (e.g. "copy '/secret/a' to '/secret/b' failed"). A single pass only
	// catches the first one, so loop until none remain. Unmatched single
	// quotes (no closing pair before the end of the string) are left in
	// place — they likely indicate the error wasn't produced by our
	// fmt.Errorf pattern and the original text is the safer rendering.
	for {
		i := strings.Index(msg, "'")
		if i < 0 {
			break
		}
		j := strings.Index(msg[i+1:], "'")
		if j < 0 {
			break
		}
		head := strings.TrimRight(msg[:i], " ")
		tail := strings.TrimLeft(msg[i+j+2:], " ")
		switch {
		case head == "":
			msg = tail
		case tail == "":
			msg = head
		default:
			msg = head + " " + tail
		}
	}
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "command failed"
	}
	return msg
}

// Run starts the interactive CLI
func (c *CLI) Run() error {
	// If username is provided without password, prompt for password
	cliConfig := c.Config
	switch cliConfig.CLIMode {
	case APIMode:
		apiConfig := c.Config.APIClientConfig.APIServerMap[c.Config.APIClientConfig.CurrentAPIServer]
		if apiConfig.UserName != nil && apiConfig.UserPassword == nil && apiConfig.APIKey == nil {
			// provider username but no password or api token
			maxAttempts := 3
			for attempt := 1; attempt <= maxAttempts; attempt++ {
				fmt.Printf("Please input your password: ")

				password, err := ReadPassword()

				if password == "" {
					if attempt < maxAttempts {
						fmt.Println("Password cannot be empty, please try again")
						continue
					}
					return errors.New("no password provided after 3 attempts")
				}

				apiConfig.UserPassword = &password

				if err = c.VerifyAuth(*apiConfig.UserName, *apiConfig.UserPassword); err != nil {
					if attempt < maxAttempts {
						fmt.Printf("Authentication failed (%d/%d attempts)\n", attempt, maxAttempts)
						continue
					}
					return fmt.Errorf("authentication failed after %d attempts", maxAttempts)
				}

				break
			}
		}

	case AdminMode:
		adminConfig := c.Config.AdminClientConfig
		if adminConfig.AdminName != nil && adminConfig.AdminPassword == nil {
			// provider username but no password or api token
			maxAttempts := 3
			for attempt := 1; attempt <= maxAttempts; attempt++ {
				fmt.Printf("Please input your password: ")

				password, err := ReadPassword()

				if password == "" {
					if attempt < maxAttempts {
						fmt.Println("Password cannot be empty, please try again")
						continue
					}
					return errors.New("no password provided after 3 attempts")
				}

				adminConfig.AdminPassword = &password

				if err = c.VerifyAuth(*adminConfig.AdminName, *adminConfig.AdminPassword); err != nil {
					if attempt < maxAttempts {
						fmt.Printf("Authentication failed (%d/%d attempts)\n", attempt, maxAttempts)
						continue
					}
					return fmt.Errorf("authentication failed after %d attempts", maxAttempts)
				}

				break
			}
		}

	default:
		return fmt.Errorf("unexpected CLI mode: %s", cliConfig.CLIMode)
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
		var prompt string
		switch cliConfig.CLIMode {
		case APIMode:
			prompt = fmt.Sprintf("RAGFlow(api/%s)> ", c.Config.APIClientConfig.CurrentAPIServer)
		case AdminMode:
			prompt = "RAGFlow(admin)> "
		default:
			return fmt.Errorf("unexpected CLI mode: %s", cliConfig.CLIMode)
		}
		input, err := c.line.Prompt(prompt)
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
			// err.Error() can include user-controlled input (e.g. dataset
			// names, file paths) via fmt.Errorf("... %s ...", userInput) in
			// the command handlers. Don't echo that back to the operator
			// verbatim — log the full error server-side for debugging, and
			// show only the error type/message via a sanitized wrapper.
			fmt.Printf("ragflow-cli error: %s\n", sanitizeCLIError(err))
		}
	}

	return nil
}

func (c *CLI) execute(input string) error {
	p := NewParser(input)
	cmd, err := p.Parse(c.Config.CLIMode)
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
	result, err = c.ExecuteCommand(cmd)
	if result != nil {
		result.PrintOut()
	}
	return err
}

func (c *CLI) handleMetaCommand(cmd *Command) error {
	command := cmd.Params["command"].(string)
	//args, _ := cmd.Params["args"].([]string)

	switch command {
	case "q", "quit", "exit":
		fmt.Println("Goodbye!")
		c.running = false
	case "?", "h", "help":
		c.printHelp()
	case "pwd":
		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		fmt.Println(dir)
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
  LOGIN USER 'email' PASSWORD 'pwd';                     - Login as user with password
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
  ADD PROVIDER 'name';                                   - Create a provider without API key
  ADD PROVIDER 'name' 'api_key';                         - Create a provider with API key
  DROP TOKEN 'token_value';                              - Delete an API token
  DELETE PROVIDER 'name';                                - Delete a provider
  SET TOKEN 'token_value';                               - Set and validate API token
  SHOW TOKEN;                                            - Show current API token
  SHOW PROVIDER 'name';                                  - Show provider details
  SHOW CURRENT MODEL;                                    - Show current model settings
  UNSET TOKEN;                                           - Remove current API token
  ALTER PROVIDER 'name' NAME 'new_name';                 - Rename a provider
  USE MODEL 'provider/instance/model';                   - Set current model for chat
  CHAT 'message';                                        - Chat using current model
  CHAT 'provider/instance/model' 'message';              - Chat with specified model
  OPENAI_CHAT 'chat_id' 'message' [options] ;            - OpenAI-compatible chat 
                                                           (run openai_chat -h for detailed options)
  CHAT COMPLETIONS 'question' [options] ;                - Chat completions via /api/v1/chat/completions
                                                           (run chat completions -h for detailed options)

Filesystem Commands (no quotes):
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
  ragflow-cli -f rf.yml "LIST USERS"           # SQL mode (with quotes)
  ragflow-cli -f rf.yml ls datasets            # Filesystem mode (no quotes)
  ragflow-cli -f rf.yml ls files               # List files in root
  ragflow-cli -f rf.yml cat datasets           # Error: datasets is a directory
  ragflow-cli -f rf.yml ls files/myfolder      # List folder contents

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

// RunSingleCommand executes a single command and exits
func (c *CLI) RunSingleCommand(command *string) error {
	// Ensure cleanup is called on exit to restore terminal settings
	defer c.Cleanup()

	// Execute the command
	if err := c.execute(*command); err != nil {
		return err
	}
	return nil
}

// VerifyAuth verifies authentication if needed
func (c *CLI) VerifyAuth(username, password string) error {
	// Otherwise, use username/password authentication
	if username == "" {
		return fmt.Errorf("username is required")
	}

	if password == "" {
		return fmt.Errorf("password is required")
	}

	// Create login command with username and password
	cmd := NewCommand("login_user_on_startup")
	cmd.Params["email"] = username
	cmd.Params["password"] = password

	_, err := c.LoginUserByCommand(cmd)
	return err
}

func (c *CLI) GetPublicKeyPEM() ([]byte, error) {

	var publicKeyFile *string = nil
	switch c.Config.CLIMode {
	case AdminMode:
		publicKeyFile = c.Config.AdminClientConfig.KeyFile
	case APIMode:
		publicKeyFile = c.Config.APIClientConfig.APIServerMap[c.Config.APIClientConfig.CurrentAPIServer].KeyFile
	}
	if publicKeyFile == nil {
		result := "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEArq9XTUSeYr2+N1h3Afl/\nz8Dse/2yD0ZGrKwx+EEEcdsBLca9Ynmx3nIB5obmLlSfmskLpBo0UACBmB5rEjBp\n2Q2f3AG3Hjd4B+gNCG6BDaawuDlgANIhGnaTLrIqWrrcm4EMzJOnAOI1fgzJRsOO\nUEfaS318Eq9OVO3apEyCCt0lOQK6PuksduOjVxtltDav+guVAA068NrPYmRNabVK\nRNLJpL8w4D44sfth5RvZ3q9t+6RTArpEtc5sh5ChzvqPOzKGMXW83C95TxmXqpbK\n6olN4RevSfVjEAgCydH6HN6OhtOQEcnrU97r9H0iZOWwbw3pVrZiUkuRD1R56Wzs\n2wIDAQAB\n-----END PUBLIC KEY-----"
		return []byte(result), nil
	}

	publicKeyPEM, err := os.ReadFile(*publicKeyFile)
	if err != nil {
		return []byte(""), fmt.Errorf("failed to read public key: %w", err)
	}
	return publicKeyPEM, nil
}

// printSearchHelp prints help for the search command
func printSearchHelp() {
	help := `Search command usage: search <query> [path] [-n number]

Search for content in datasets or skills.

Arguments:
  <query>                Search query (required)
                         Example: "machine learning"
  [path]                 Path to search in (default: datasets)
                         Supports:
                           - 'datasets' (all datasets)
                           - 'datasets/<kb_name>' (specific dataset)
                           - 'skills' (default skill space)
                           - 'skills/<space_name>' (specific skill space)
                         Example: skills/space1

Options:
  -n, --number <num>     Number of results to return (default: 10)
                         Example: -n 20
  -h, --help             Show this help message

Output:
  Default output format is JSON. Use --output plain or --output table for other formats.

Examples:
  search "neural networks"                          # Search all datasets
  search "AI" datasets/kb1                          # Search in kb1
  search "RAG" skills/space1 -n 20                    # Search skills in hub1, return 20 results
  search "data processing" skills                   # Search skills (default space)

Datasets syntax (full filter set):
  search 'query' on datasets 'kb_names' [with <option> <value> ...] [;]

  'kb_names' is a single quoted string. Pass one name for a single
  dataset, or a comma-separated list (no spaces) to search across
  multiple datasets in one call:
    'kb1'                  # one dataset
    'kb1,kb2'              # two datasets
    'kb1,kb2,kb3'          # three datasets

  When 'on datasets' is given, the search runs against the named
  dataset(s) and accepts the full WITH-option set below.

  WITH options (space-separated, not comma-separated):
    top_k                   <int>     Number of results (default 5)
    page_size               <int>     Page size for pagination
    page                    <int>     Page number (1-based)
    similarity_threshold    <float>   Minimum similarity score (0.0-1.0)
    vector_similarity_weight <float>  Weight given to vector vs text score
    keyword                 true|false  Enable keyword extraction via LLM
    use_kg                  true|false  Enable knowledge-graph augmentation
    rerank_id               'id'      Rerank model to apply
    tenant_rerank_id        'id'      Tenant-scoped rerank model
    search_id               'id'      Idempotency / search-session id
    meta_data_filter        '<json>'  Metadata filter (must be valid JSON)
    cross_languages         ['a','b'] Source languages to translate from
    doc_ids                 ['d1',...] Restrict to specific document ids

  Examples:
    search 'AI' on datasets 'kb_chinese' with top_k 10;
    search 'AI' on datasets 'kb1' 'kb2' with top_k 20 similarity_threshold 0.3 cross_languages ['Chinese']
        doc_ids ['d1', 'd2'];
    search 'manual' on datasets 'kb1' with
        meta_data_filter '{"method":"manual","conditions":[{"key":"author","op":"eq","value":"Luo"}]}';
`
	fmt.Println(help)
}

// printOpenaiChatHelp prints help for the OPENAI_CHAT command.
func printOpenaiChatHelp() {
	help := `OPENAI_CHAT — hit POST /api/v1/openai/<chat_id>/chat/completions

Syntax:
  OPENAI_CHAT 'chat_id' 'message'
       [system "..."]
       [history "user:...;assistant:...;user:..."]
       [history_delimiter "<char>"]
       [model <string>]
       [temperature <float>] [max_tokens <int>] [stream <bool>]
       [top_p <float>] [frequency_penalty <float>] [presence_penalty <float>]
       [extra_body <json>] ;

Required positional:
  'chat_id'   the dialog id (becomes the URL path segment)
  'message'   the user message content

Named options (any order; all optional with defaults):
  system            '...'           override the system prompt
  history           '...'           prior turns: user:...;assistant:...;user:...
  history_delimiter '...'           turn separator for history (default ';')
  model             '...'           'model' (sentinel) or composite (default 'model')
  temperature       <float>         0..2  (default 0)
  max_tokens        <int>           (default 0 = server/model default)
  stream            <bool>          true|false  (default false)
  top_p             <float>         0..1
  frequency_penalty <float>         -2..2
  presence_penalty  <float>         -2..2
  extra_body        <json>          '{"reference":true,...}'

Defaults:
  model       'model'  — server resolves to the dialog's configured LLM
  stream      false
  temperature 0
  history_delimiter ';'      — commas in content survive unchanged

extra_body allowlist:
  reference            bool
  reference_metadata   { include?: bool, fields?: string[] }
  metadata_condition   { logic?: "and"|"or", conditions?: [{key, operator, value}] }

Examples:
  OPENAI_CHAT 'cid' 'Hello, how are you?';
  OPENAI_CHAT 'cid' 'Hello' model 'Qwen/Qwen3-8B@ling@SILICONFLOW' temperature 0.7 max_tokens 512;
  OPENAI_CHAT 'cid' 'Hello' stream true;
  OPENAI_CHAT 'cid' 'next' system 'You are concise.' history 'user:q1;assistant:a1';
  OPENAI_CHAT 'cid' 'Hello' extra_body '{"reference":true,"metadata_condition":{"logic":"and","conditions":[{"key":"doc_type","operator":"is","value":"faq"}]}}';
`
	fmt.Println(help)
}

// printChatCompletionsHelp prints help for the CHAT COMPLETIONS command.
func printChatCompletionsHelp() {
	help := `CHAT COMPLETIONS — hit POST /api/v1/chat/completions

Syntax:
  CHAT COMPLETIONS 'question'
       chat_id '...'
       [session "..."] [llm "..."]
       [system "..."] [history "..."] [history_delimiter "<char>"]
       [temperature <float>] [max_tokens <int>] [stream <bool>]
       [top_p <float>] [frequency_penalty <float>] [presence_penalty <float>]
       [pass_all_history <bool>] [legacy <bool>] ;

Required positional:
  'question'  the user question

Named options (any order; all optional with defaults):
  chat_id           '...'  the dialog id (optional)
  session           '...'  existing session/conversation id
  llm               '...'  override the dialog's LLM
  system            '...'  override the system prompt
  history           '...'  prior turns: user:...;assistant:...;user:...
  history_delimiter '...'  turn separator for history (default ';')
  temperature       <float>  0..2  (default 0)
  max_tokens        <int>    (default 0 = server/model default)
  stream            <bool>   true|false  (default false)
  top_p             <float>  0..1
  frequency_penalty <float>  -2..2
  presence_penalty  <float>  -2..2
  pass_all_history  <bool>   pass all history messages
  legacy            <bool>   use legacy SSE format

Defaults:
  stream            false
  temperature       0
  history_delimiter ';'

Examples:
  CHAT COMPLETIONS 'Hello, how are you?' chat_id 'cid';
  CHAT COMPLETIONS 'Explain quantum computing' chat_id 'cid' stream true;
  CHAT COMPLETIONS 'Next question' chat_id 'cid' session 'sess-abc123';
  CHAT COMPLETIONS 'What about X?' chat_id 'cid' system 'You are a helpful assistant.' history 'user:Tell me about Y;assistant:Y is...';
  CHAT COMPLETIONS 'Summarize' chat_id 'cid' llm 'Qwen/Qwen3-8B@ling@SILICONFLOW' temperature 0.7 max_tokens 512;
`
	fmt.Println(help)
}
