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
	"fmt"
	"io"
	"os"
	"ragflow/internal/cli/filesystem"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ExecuteFilesystemCommand ExecuteFilesystem executes a Filesystem command and returns a ResponseIf.
func (c *CLI) ExecuteFilesystemCommand(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

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
