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
	"io"
	"os"
	//"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	//"syscall"
	"unicode/utf8"

	"github.com/peterh/liner"
	"gopkg.in/yaml.v3"

	"ragflow/internal/cli/filesystem"
)

type APIServerConfig struct {
	Name         string  `yaml:"name"`
	Host         string  `yaml:"host"`
	UserName     *string `yaml:"user_name"`
	UserPassword *string `yaml:"password"`
	ApiToken     *string `yaml:"api_token"`
	IP           string
	Port         int
}

// ConfigFile represents the rf.yml configuration file structure
type ConfigFile struct {
	Host         string                      `yaml:"host"`      // default API server host
	APIToken     string                      `yaml:"api_token"` // default API server api token
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
			ApiToken:     nil,
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
					defaultApiServerConfig.ApiToken = &args[i+1]
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
			if config.APIToken != "" {
				if defaultApiServerConfig.ApiToken == nil {
					defaultApiServerConfig.ApiToken = &config.APIToken
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

Usage: ragflow_cli [options] [command]

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
		if apiServerConfig.ApiToken != nil {
			httpClient.APIToken = apiServerConfig.ApiToken
			httpClient.useAPIToken = true
		}
		cli.APIServerClientMap = map[string]*HTTPClient{
			cli.Config.APIClientConfig.CurrentAPIServer: httpClient,
		}
		// Auto-login if user and password are provided (from config file)
		if apiServerConfig.UserName != nil && apiServerConfig.UserPassword != nil && apiServerConfig.ApiToken == nil {
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

// Run starts the interactive CLI
func (c *CLI) NewRun() error {
	// If username is provided without password, prompt for password
	cliConfig := c.Config
	switch cliConfig.CLIMode {
	case APIMode:
		apiConfig := c.Config.APIClientConfig.APIServerMap[c.Config.APIClientConfig.CurrentAPIServer]
		if apiConfig.UserName != nil && apiConfig.UserPassword == nil && apiConfig.ApiToken == nil {
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

		if err = c.executeNew(input); err != nil {
			fmt.Printf("CLI error: %v\n", err)
		}
	}

	return nil
}

func (c *CLI) executeNew(input string) error {
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

// executeFilesystem executes a Filesystem command and returns a ResponseIf.
func (c *CLI) executeFilesystem(cmd *Command) (ResponseIf, error) {
	rawInput, _ := cmd.Params["command"].(string)

	r, w, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = old
		_ = w.Close()
		_ = r.Close()
	}()

	var buf strings.Builder
	copyErrCh := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(&buf, r)
		copyErrCh <- copyErr
	}()

	execErr := c.executeFilesystemInner(rawInput)
	_ = w.Close() // signal EOF to reader goroutine
	copyErr := <-copyErrCh
	if copyErr != nil {
		return nil, fmt.Errorf("capture filesystem output: %w", copyErr)
	}
	return &FileSystemResponse{Output: buf.String()}, execErr
}

// executeFilesystemInner executes a Filesystem command and writes output to stdout.
// It is called by executeFilesystem which captures the stdout output.
func (c *CLI) executeFilesystemInner(input string) error {
	// Parse input into arguments
	var args []string
	// Interactive mode: parse input
	args = parseFilesystemArgs(input)

	if len(args) == 0 {
		return fmt.Errorf("no command provided")
	}

	// Check if we have a filesystem engine
	if c.ContextEngine == nil {
		return fmt.Errorf("filesystem engine not available")
	}

	cmdType := args[0]
	cmdArgs := args[1:]

	// Build filesystem command
	var ceCmd *filesystem.Command

	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]

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
		ceCmd = &filesystem.Command{
			Type: filesystem.CommandList,
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
		// Check if searching skills (supports: "skills" or "skills/space1")
		if searchPath == "skills" || strings.HasPrefix(searchPath, "skills/") {
			// Parse space ID from path (e.g., "skills/space1" -> "space1")
			spaceID := "default"
			if strings.HasPrefix(searchPath, "skills/") {
				spaceID = strings.TrimPrefix(searchPath, "skills/")
				if spaceID == "" {
					spaceID = "default"
				}
			}
			// Get skill provider and perform search
			provider := c.ContextEngine.GetProvider("skills")
			if provider == nil {
				return fmt.Errorf("skill provider not available")
			}
			skillProvider, ok := provider.(*filesystem.SkillProvider)
			if !ok {
				return fmt.Errorf("invalid skill provider type")
			}
			pageSize := searchOpts.TopK
			if pageSize <= 0 {
				pageSize = 10
			}
			searchOptions := &filesystem.SearchOptions{
				Query:  searchOpts.Query,
				Limit:  pageSize,
				Offset: 0,
				TopK:   pageSize,
			}
			result, err := skillProvider.Search(context.Background(), spaceID, searchOptions)
			if err != nil {
				return err
			}
			// Print skill search results with full details
			c.printSkillSearchResults(result, c.Config.OutputFormat)
			return nil
		}
		ceCmd = &filesystem.Command{
			Type: filesystem.CommandSearch,
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
		content, err := c.ContextEngine.Cat(context.Background(), cmdArgs[0])
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
	case "install-skill":
		// Get the file provider and skill provider from the engine
		fileProvider, ok := c.ContextEngine.GetProvider("files").(*filesystem.FileProvider)
		if !ok {
			return fmt.Errorf("file provider not available")
		}
		skillProvider := c.ContextEngine.GetProvider("skills")
		if skillProvider == nil {
			return fmt.Errorf("skill provider not available")
		}
		// Create adapter for HTTPClient
		httpAdapter := &httpClientAdapter{client: httpClient}
		cmd := filesystem.NewInstallSkillCommand(httpAdapter, fileProvider, skillProvider)
		return cmd.Execute(cmdArgs)
	case "uninstall-skill":
		skillProvider := c.ContextEngine.GetProvider("skills")
		if skillProvider == nil {
			return fmt.Errorf("skill provider not available")
		}
		fileProvider := c.ContextEngine.GetProvider("files")
		if fileProvider == nil {
			return fmt.Errorf("file provider not available")
		}
		// Create adapter for HTTPClient
		httpAdapter := &httpClientAdapter{client: httpClient}
		fileProv, _ := fileProvider.(*filesystem.FileProvider)
		cmd := filesystem.NewUninstallSkillCommand(httpAdapter, skillProvider, fileProv)
		return cmd.Execute(cmdArgs)
	default:
		return fmt.Errorf("unknown filesystem command: %s", cmdType)
	}

	// Execute the command
	result, err := c.ContextEngine.Execute(context.Background(), ceCmd)
	if err != nil {
		return err
	}

	// Print result
	// For search command, default to JSON format if not explicitly set to plain/table
	format := c.Config.OutputFormat
	if ceCmd.Type == filesystem.CommandSearch && format != OutputFormatPlain && format != OutputFormatTable {
		format = OutputFormatJSON
	}
	// Get limit for list command
	limit := 0
	if ceCmd.Type == filesystem.CommandList {
		if l, ok := ceCmd.Params["limit"].(int); ok {
			limit = l
		}
	}
	c.printFilesystemResult(result, ceCmd.Type, format, limit)
	return nil
}

// parseFilesystemArgs parses Filesystem command arguments
// Supports simple space-separated args and quoted strings
func parseFilesystemArgs(input string) []string {
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

// printFilesystemResult prints the result of a filesystem command
func (c *CLI) printFilesystemResult(result *filesystem.Result, cmdType filesystem.CommandType, format OutputFormat, limit int) {
	if result == nil {
		return
	}

	switch cmdType {
	case filesystem.CommandList:
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
	case filesystem.CommandSearch:
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
	case filesystem.CommandCat:
		// Cat output is handled differently - it returns []byte, not *Result
		// This case should not be reached in normal flow since Cat returns []byte directly
		fmt.Println("Content retrieved")
	}
}

// printSkillSearchResults prints skill search results with full details
func (c *CLI) printSkillSearchResults(result *filesystem.Result, format OutputFormat) {
	if result == nil || len(result.Nodes) == 0 {
		if format == OutputFormatJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("No skills found")
		}
		return
	}

	// Skill search result structure
	type skillSearchResult struct {
		SkillID     string  `json:"skill_id"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Tags        string  `json:"tags"`
		Score       float64 `json:"score"`
		BM25Score   float64 `json:"bm25_score"`
		VectorScore float64 `json:"vector_score"`
	}

	results := make([]skillSearchResult, 0, len(result.Nodes))
	for _, node := range result.Nodes {
		// Extract metadata
		skillID := ""
		if id, ok := node.Metadata["skill_id"].(string); ok {
			skillID = id
		}
		description := ""
		if desc, ok := node.Metadata["description"].(string); ok {
			description = desc
		}
		tags := ""
		if t, ok := node.Metadata["tags"].([]string); ok {
			tags = strings.Join(t, ", ")
		}
		var score, bm25Score, vectorScore float64
		if s, ok := node.Metadata["score"].(float64); ok {
			score = s
		}
		if b, ok := node.Metadata["bm25_score"].(float64); ok {
			bm25Score = b
		}
		if v, ok := node.Metadata["vector_score"].(float64); ok {
			vectorScore = v
		}

		results = append(results, skillSearchResult{
			SkillID:     skillID,
			Name:        node.Name,
			Description: description,
			Tags:        tags,
			Score:       score,
			BM25Score:   bm25Score,
			VectorScore: vectorScore,
		})
	}

	if format == OutputFormatJSON {
		jsonData, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			fmt.Printf("Error marshaling JSON: %v\n", err)
			return
		}
		fmt.Println(string(jsonData))
	} else if format == OutputFormatPlain {
		fmt.Printf("Found %d skill(s):\n", len(results))
		for _, sr := range results {
			fmt.Printf("\nName: %s\n", sr.Name)
			fmt.Printf("Skill ID: %s\n", sr.SkillID)
			fmt.Printf("Description: %s\n", sr.Description)
			fmt.Printf("Tags: %s\n", sr.Tags)
			fmt.Printf("Score: %.6f (BM25: %.6f, Vector: %.6f)\n", sr.Score, sr.BM25Score, sr.VectorScore)
		}
	} else {
		// Table format
		fmt.Printf("Found %d skill(s):\n", len(results))
		fmt.Println()
		for _, sr := range results {
			fmt.Printf("Name:        %s\n", sr.Name)
			fmt.Printf("Skill ID:    %s\n", sr.SkillID)
			fmt.Printf("Description: %s\n", sr.Description)
			fmt.Printf("Tags:        %s\n", sr.Tags)
			fmt.Printf("Score:       %.6f (BM25: %.6f, Vector: %.6f)\n", sr.Score, sr.BM25Score, sr.VectorScore)
			fmt.Println()
		}
	}
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
  ragflow_cli -f rf.yml "LIST USERS"           # SQL mode (with quotes)
  ragflow_cli -f rf.yml ls datasets            # Filesystem mode (no quotes)
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
func (c *CLI) NewVerifyAuth(username, password *string) error {
	// Otherwise, use username/password authentication
	if username == nil {
		return fmt.Errorf("username is required")
	}

	if password == nil {
		return fmt.Errorf("password is required")
	}

	// Create login command with username and password
	cmd := NewCommand("login_user")
	cmd.Params["email"] = *username
	cmd.Params["password"] = *password
	_, err := c.ExecuteCommand(cmd)
	return err
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
	cmd := NewCommand("login_user")
	cmd.Params["email"] = username
	cmd.Params["password"] = password
	_, err := c.ExecuteCommand(cmd)
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
// Format: search <query> [path] [-n number]
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
	// Format: search <query> [path] [-n number]
	i := 0
	for i < len(args) {
		arg := args[i]

		// Handle -n flag for number of results
		if arg == "-n" || arg == "--number" {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for %s flag", arg)
			}
			topK, err := strconv.Atoi(args[i+1])
			if err != nil {
				return nil, fmt.Errorf("invalid number value: %s", args[i+1])
			}
			opts.TopK = topK
			i += 2
			continue
		}

		// If it starts with -, it's an unknown flag
		if strings.HasPrefix(arg, "-") {
			return nil, fmt.Errorf("unknown flag: %s", arg)
		}

		// Non-flag arguments: first is query, second is path
		if opts.Query == "" {
			opts.Query = arg
		} else if len(opts.Dirs) == 0 {
			opts.Dirs = append(opts.Dirs, arg)
		}
		i++
	}

	// Validate required parameters
	if opts.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	// If no path specified, default to "datasets"
	if len(opts.Dirs) == 0 {
		opts.Dirs = []string{"datasets"}
	}

	return opts, nil
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
