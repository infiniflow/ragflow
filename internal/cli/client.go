package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

// RAGFlowClient handles API interactions with the RAGFlow server
type RAGFlowClient struct {
	HTTPClient *HTTPClient
	ServerType string // "admin" or "user"
}

// NewRAGFlowClient creates a new RAGFlow client
func NewRAGFlowClient(serverType string) *RAGFlowClient {
	return &RAGFlowClient{
		HTTPClient: NewHTTPClient(),
		ServerType: serverType,
	}
}

// LoginUser performs user login
func (c *RAGFlowClient) LoginUser(cmd *Command) error {
	// First, ping the server to check if it's available
	resp, err := c.HTTPClient.Request("GET", "/system/ping", false, "web", nil, nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("Can't access server for login (connection failed)")
		return err
	}

	if resp.StatusCode != 200 || string(resp.Body) != "pong" {
		fmt.Println("Server is down")
		return fmt.Errorf("server is down")
	}

	email, ok := cmd.Params["email"].(string)
	if !ok {
		return fmt.Errorf("email not provided")
	}

	// Get password from user input (hidden)
	fmt.Printf("password for %s: ", email)
	password, err := readPassword()
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	password = strings.TrimSpace(password)

	// Login
	token, err := c.loginUser(email, password)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("Can't access server for login (connection failed)")
		return err
	}

	c.HTTPClient.LoginToken = token
	fmt.Printf("Login user %s successfully\n", email)
	return nil
}

// loginUser performs the actual login request
func (c *RAGFlowClient) loginUser(email, password string) (string, error) {
	// Encrypt password using scrypt (same as Python implementation)
	encryptedPassword, err := EncryptPassword(password)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt password: %w", err)
	}

	payload := map[string]interface{}{
		"email":    email,
		"password": encryptedPassword,
	}

	var path string
	if c.ServerType == "admin" {
		path = "/admin/login"
	} else {
		path = "/user/login"
	}

	resp, err := c.HTTPClient.Request("POST", path, c.ServerType == "admin", "", nil, payload)
	if err != nil {
		return "", err
	}

	resJSON, err := resp.JSON()
	if err != nil {
		return "", fmt.Errorf("login failed: invalid JSON response (%w)", err)
	}

	code, ok := resJSON["code"].(float64)
	if !ok || code != 0 {
		msg, _ := resJSON["message"].(string)
		return "", fmt.Errorf("login failed: %s", msg)
	}

	token := resp.Headers.Get("Authorization")
	if token == "" {
		return "", fmt.Errorf("login failed: missing Authorization header")
	}

	return token, nil
}

// PingServer pings the server to check if it's alive
// Returns benchmark result map if iterations > 1, otherwise prints status
func (c *RAGFlowClient) PingServer(cmd *Command) (map[string]interface{}, error) {
	// Get iterations from command params (for benchmark)
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	if iterations > 1 {
		// Benchmark mode: multiple iterations
		result, err := c.HTTPClient.RequestWithIterations("GET", "/system/ping", false, "web", nil, nil, iterations)
		if err != nil {
			return nil, err
		}
		return result, nil
	}

	// Single ping mode
	resp, err := c.HTTPClient.Request("GET", "/system/ping", false, "web", nil, nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("Server is down")
		return nil, err
	}

	if resp.StatusCode == 200 && string(resp.Body) == "pong" {
		fmt.Println("Server is alive")
	} else {
		fmt.Printf("Error: %d\n", resp.StatusCode)
	}
	return nil, nil
}

// ListUserDatasets lists datasets for current user (user mode)
// Returns (result_map, error) - result_map is non-nil for benchmark mode
func (c *RAGFlowClient) ListUserDatasets(cmd *Command) (map[string]interface{}, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	// Check for benchmark iterations
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return c.HTTPClient.RequestWithIterations("POST", "/kb/list", false, "web", nil, nil, iterations)
	}

	// Normal mode
	resp, err := c.HTTPClient.Request("POST", "/kb/list", false, "web", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list datasets: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list datasets: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	resJSON, err := resp.JSON()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	code, ok := resJSON["code"].(float64)
	if !ok || code != 0 {
		msg, _ := resJSON["message"].(string)
		return nil, fmt.Errorf("failed to list datasets: %s", msg)
	}

	data, ok := resJSON["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	kbs, ok := data["kbs"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format: kbs not found")
	}

	// Convert to slice of maps
	tableData := make([]map[string]interface{}, 0, len(kbs))
	for _, kb := range kbs {
		if kbMap, ok := kb.(map[string]interface{}); ok {
			// Remove avatar field
			delete(kbMap, "avatar")
			tableData = append(tableData, kbMap)
		}
	}

	PrintTableSimple(tableData)
	return nil, nil
}

// ListDatasets lists datasets for a specific user (admin mode)
// Returns (result_map, error) - result_map is non-nil for benchmark mode
func (c *RAGFlowClient) ListDatasets(cmd *Command) (map[string]interface{}, error) {
	if c.ServerType != "admin" {
		return nil, fmt.Errorf("this command is only allowed in ADMIN mode")
	}

	userName, ok := cmd.Params["user_name"].(string)
	if !ok {
		return nil, fmt.Errorf("user_name not provided")
	}

	// Check for benchmark iterations
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return c.HTTPClient.RequestWithIterations("GET", fmt.Sprintf("/admin/users/%s/datasets", userName), true, "admin", nil, nil, iterations)
	}

	fmt.Printf("Listing all datasets of user: %s\n", userName)

	resp, err := c.HTTPClient.Request("GET", fmt.Sprintf("/admin/users/%s/datasets", userName), true, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list datasets: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list datasets: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	resJSON, err := resp.JSON()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	data, ok := resJSON["data"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	// Convert to slice of maps and remove avatar
	tableData := make([]map[string]interface{}, 0, len(data))
	for _, item := range data {
		if itemMap, ok := item.(map[string]interface{}); ok {
			delete(itemMap, "avatar")
			tableData = append(tableData, itemMap)
		}
	}

	PrintTableSimple(tableData)
	return nil, nil
}

// readPassword reads password from terminal without echoing
func readPassword() (string, error) {
	// Check if stdin is a terminal by trying to get terminal size
	if isTerminal() {
		// Use stty to disable echo
		cmd := exec.Command("stty", "-echo")
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			// Fallback: read normally
			return readPasswordFallback()
		}
		defer func() {
			// Re-enable echo
			cmd := exec.Command("stty", "echo")
			cmd.Stdin = os.Stdin
			cmd.Run()
		}()

		reader := bufio.NewReader(os.Stdin)
		password, err := reader.ReadString('\n')
		fmt.Println() // New line after password input
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(password), nil
	}

	// Fallback for non-terminal input (e.g., piped input)
	return readPasswordFallback()
}

// isTerminal checks if stdin is a terminal
func isTerminal() bool {
	var termios syscall.Termios
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&termios)), 0, 0, 0)
	return err == 0
}

// readPasswordFallback reads password as plain text (fallback mode)
func readPasswordFallback() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	password, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(password), nil
}

// SearchOnDatasets searches for chunks in specified datasets
// Returns (result_map, error) - result_map is non-nil for benchmark mode
func (c *RAGFlowClient) SearchOnDatasets(cmd *Command) (map[string]interface{}, error) {
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	question, ok := cmd.Params["question"].(string)
	if !ok {
		return nil, fmt.Errorf("question not provided")
	}

	datasets, ok := cmd.Params["datasets"].(string)
	if !ok {
		return nil, fmt.Errorf("datasets not provided")
	}

	// Parse dataset IDs (comma-separated)
	datasetIDs := strings.Split(datasets, ",")
	for i := range datasetIDs {
		datasetIDs[i] = strings.TrimSpace(datasetIDs[i])
	}

	// Check for benchmark iterations
	iterations := 1
	if val, ok := cmd.Params["iterations"].(int); ok && val > 1 {
		iterations = val
	}

	payload := map[string]interface{}{
		"kb_id":                    datasetIDs,
		"question":                 question,
		"similarity_threshold":     0.2,
		"vector_similarity_weight": 0.3,
	}

	if iterations > 1 {
		// Benchmark mode - return raw result for benchmark stats
		return c.HTTPClient.RequestWithIterations("POST", "/chunk/retrieval_test", false, "web", nil, payload, iterations)
	}

	// Normal mode
	resp, err := c.HTTPClient.Request("POST", "/chunk/retrieval_test", false, "web", nil, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to search on datasets: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to search on datasets: HTTP %d, body: %s", resp.StatusCode, string(resp.Body))
	}

	resJSON, err := resp.JSON()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	code, ok := resJSON["code"].(float64)
	if !ok || code != 0 {
		msg, _ := resJSON["message"].(string)
		return nil, fmt.Errorf("failed to search on datasets: %s", msg)
	}

	data, ok := resJSON["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	chunks, ok := data["chunks"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format: chunks not found")
	}

	// Convert to slice of maps for printing
	tableData := make([]map[string]interface{}, 0, len(chunks))
	for _, chunk := range chunks {
		if chunkMap, ok := chunk.(map[string]interface{}); ok {
			row := map[string]interface{}{
				"id":          chunkMap["id"],
				"content":     chunkMap["content"],
				"document_id": chunkMap["document_id"],
				"dataset_id":  chunkMap["dataset_id"],
				"similarity":  chunkMap["similarity"],
			}
			tableData = append(tableData, row)
		}
	}

	PrintTableSimple(tableData)
	return nil, nil
}

// ExecuteCommand executes a parsed command
// Returns benchmark result map for commands that support it (e.g., ping_server with iterations > 1)
func (c *RAGFlowClient) ExecuteCommand(cmd *Command) (map[string]interface{}, error) {
	switch cmd.Type {
	case "login_user":
		return nil, c.LoginUser(cmd)
	case "ping_server":
		return c.PingServer(cmd)
	case "benchmark":
		return nil, c.RunBenchmark(cmd)
	case "list_user_datasets":
		return c.ListUserDatasets(cmd)
	case "list_datasets":
		return c.ListDatasets(cmd)
	case "search_on_datasets":
		return c.SearchOnDatasets(cmd)
	// TODO: Implement other commands
	default:
		return nil, fmt.Errorf("command '%s' would be executed with API", cmd.Type)
	}
}
