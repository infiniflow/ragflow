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

// ExecuteCommand executes a parsed command
// Returns benchmark result map for commands that support it (e.g., ping_server with iterations > 1)
func (c *RAGFlowClient) ExecuteCommand(cmd *Command) (map[string]interface{}, error) {
	switch cmd.Type {
	case "login_user":
		return nil, c.LoginUser(cmd)
	case "ping_server":
		return c.PingServer(cmd)
	// TODO: Implement other commands
	default:
		return nil, fmt.Errorf("command '%s' would be executed with API", cmd.Type)
	}
}
