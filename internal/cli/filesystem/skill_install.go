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

package filesystem

import (
	stdctx "context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/publicsuffix"

	"ragflow/internal/cli/filesystem/skill_hub/security"
	"ragflow/internal/cli/filesystem/skill_hub/source"
)

// InstallSkillArgs holds the parsed arguments for install-skill command
type InstallSkillArgs struct {
	SpaceID    string // Target skills space ID
	SourceRef  string // Source reference (path or identifier)
	Version    string // Skill version
	SkillName  string // Optional: override skill name
	Force      bool   // Force reinstall
	SkipVerify bool   // Skip security verification
	ShowHelp   bool
}

// SkillInstallCommand handles the install-skill command
type SkillInstallCommand struct {
	client         HTTPClientInterface
	fileProvider   *FileProvider
	skillProvider  Provider
	scanner        *security.Scanner
	guard          *security.Guard
	sourceResolver *source.SourceResolver
}

// sourceHTTPClientAdapter adapts filesystem.HTTPClientInterface to source.HTTPClientInterface
// This allows us to use the existing HTTP client infrastructure with the source package
type sourceHTTPClientAdapter struct {
	client HTTPClientInterface
	httpClient *http.Client
}

func (a *sourceHTTPClientAdapter) Do(req *http.Request) (*http.Response, error) {
	// Use standard http.Client for direct requests (e.g., GitHub API)
	// This bypasses the RAGFlow API client which adds its own base URL
	return a.httpClient.Do(req)
}

func (a *sourceHTTPClientAdapter) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return a.Do(req)
}

// NewInstallSkillCommand creates a new install-skill command handler
func NewInstallSkillCommand(client HTTPClientInterface, fileProvider *FileProvider, skillProvider Provider) *SkillInstallCommand {
	// Log proxy settings
	if httpProxy := os.Getenv("http_proxy"); httpProxy != "" {
		fmt.Printf("Using HTTP proxy: %s\n", httpProxy)
	}
	if httpsProxy := os.Getenv("https_proxy"); httpsProxy != "" {
		fmt.Printf("Using HTTPS proxy: %s\n", httpsProxy)
	}

	// Create transport with HTTP/2 support and connection reuse
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		// Enable connection pooling
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		// Enable keep-alive
		DisableKeepAlives: false,
		ForceAttemptHTTP2: true,
	}
	// Enable HTTP/2
	http2.ConfigureTransport(transport)

	// Check what proxy will be used
	testURL, _ := url.Parse("https://github.com")
	if proxy, err := transport.Proxy(&http.Request{URL: testURL}); err == nil && proxy != nil {
		fmt.Printf("Proxy enabled for GitHub: %s\n", proxy.String())
	} else if err != nil {
		fmt.Printf("Warning: proxy detection error: %v\n", err)
	}

	// Create cookie jar for session persistence
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	if err != nil {
		fmt.Printf("Warning: failed to create cookie jar: %v\n", err)
		jar = nil
	}

	// Wrap client with adapter - use standard http.Client with timeout for direct external requests
	adaptedClient := &sourceHTTPClientAdapter{
		client: client,
		httpClient: &http.Client{
			Timeout:       60 * time.Second,
			Transport:     transport,
			Jar:           jar,
		},
	}

	return &SkillInstallCommand{
		client:         client,
		fileProvider:   fileProvider,
		skillProvider:  skillProvider,
		scanner:        security.NewScanner(),
		guard:          security.NewGuard(),
		sourceResolver: source.NewSourceResolver(adaptedClient),
	}
}

// Execute runs the install-skill command
func (c *SkillInstallCommand) Execute(args []string) error {
	parsedArgs, err := c.parseArgs(args)
	if err != nil {
		return err
	}

	if parsedArgs.ShowHelp {
		c.PrintHelp()
		return nil
	}

	ctx := stdctx.Background()

	// 1. Resolve source
	fmt.Printf("Resolving source reference: %s\n", parsedArgs.SourceRef)
	src, identifier, err := c.sourceResolver.Resolve(parsedArgs.SourceRef)
	if err != nil {
		return fmt.Errorf("invalid source reference: %w", err)
	}

	// 2. Fetch skill bundle
	// If version specified, append to identifier for sources that support it
	fetchIdentifier := identifier
	if parsedArgs.Version != "" {
		fetchIdentifier = fmt.Sprintf("%s@%s", identifier, parsedArgs.Version)
		fmt.Printf("Fetching skill from %s (version %s)...\n", src.SourceID(), parsedArgs.Version)
	} else {
		fmt.Printf("Fetching skill from %s...\n", src.SourceID())
	}
	bundle, err := src.Fetch(fetchIdentifier)
	if err != nil {
		return fmt.Errorf("failed to fetch skill: %w", err)
	}
	fmt.Printf("Found skill '%s' (v%s) with %d files\n",
		bundle.Name, bundle.Metadata.Version, len(bundle.Files))

	// Override skill name if specified
	if parsedArgs.SkillName != "" {
		bundle.Name = parsedArgs.SkillName
	}

	// 3. Check if skill already exists
	exists, err := c.skillExists(ctx, parsedArgs.SpaceID, bundle.Name)
	if err != nil {
		return fmt.Errorf("failed to check existing skill: %w", err)
	}

	if exists && !parsedArgs.Force {
		return fmt.Errorf("skill '%s' already exists in space '%s'. Use --force to reinstall", bundle.Name, parsedArgs.SpaceID)
	}

	// 4. Security scan (unless skipped)
	if !parsedArgs.SkipVerify {
		fmt.Println("Running security scan...")
		trustLevel := src.TrustLevel(identifier)
		scanResult := c.scanner.ScanSkill(bundle.Name, src.SourceID(), trustLevel, bundle.Files)

		allowed, reason := c.guard.ShouldAllowInstall(scanResult, parsedArgs.Force)
		if !allowed {
			fmt.Println(c.guard.FormatScanReport(scanResult))
			return fmt.Errorf("installation blocked: %s", reason)
		}

		fmt.Println(c.guard.FormatScanReport(scanResult))
		fmt.Printf("✓ Security check passed: %s\n\n", reason)
	}

	// 5. Force mode: delete existing skill first
	if parsedArgs.Force && exists {
		fmt.Printf("Force mode: removing existing skill '%s'...\n", bundle.Name)
		if err := c.uninstallSkill(ctx, parsedArgs.SpaceID, bundle.Name); err != nil {
			return fmt.Errorf("failed to remove existing skill: %w", err)
		}
		fmt.Println()
	}

	// 6. Install skill
	fmt.Printf("Installing skill '%s' to space '%s'...\n", bundle.Name, parsedArgs.SpaceID)
	if err := c.installSkill(ctx, parsedArgs.SpaceID, bundle, parsedArgs.Force); err != nil {
		return fmt.Errorf("failed to install skill: %w", err)
	}

	// 7. Update index
	fmt.Printf("Updating search index for skill '%s'...\n", bundle.Name)
	if err := c.updateIndex(ctx, parsedArgs.SpaceID, bundle.Name); err != nil {
		fmt.Printf("⚠ Warning: failed to update index: %v\n", err)
	}

	fmt.Printf("✓ Successfully installed skill '%s' (version: %s)\n", bundle.Name, bundle.Metadata.Version)
	return nil
}

// uninstallSkill removes an existing skill (for --force mode)
func (c *SkillInstallCommand) uninstallSkill(ctx stdctx.Context, spaceID, skillName string) error {
	var indexErr, folderErr error

	// Delete index
	if skillProv, ok := c.skillProvider.(*SkillProvider); ok {
		if err := skillProv.DeleteSkill(ctx, spaceID, skillName); err != nil {
			indexErr = fmt.Errorf("failed to delete search index: %w", err)
			fmt.Printf("⚠ Warning: %v\n", indexErr)
		} else {
			fmt.Printf("✓ Search index deleted\n")
		}
	}

	// Delete folder
	if c.fileProvider != nil {
		folderPath := fmt.Sprintf("skills/%s/%s", spaceID, skillName)
		if err := c.fileProvider.DeleteFolderByPath(ctx, folderPath); err != nil {
			folderErr = fmt.Errorf("failed to delete skill folder: %w", err)
			fmt.Printf("⚠ Warning: %v\n", folderErr)
		} else {
			fmt.Printf("✓ Skill folder deleted\n")
		}
	}

	// Return error if both failed
	if indexErr != nil && folderErr != nil {
		return fmt.Errorf("failed to uninstall: index (%v), folder (%v)", indexErr, folderErr)
	}

	return nil
}

// installSkill installs a skill bundle using existing SkillUploader
func (c *SkillInstallCommand) installSkill(ctx stdctx.Context, spaceID string, bundle *source.SkillBundle, force bool) error {
	// Create a temporary directory to hold the skill files
	tempDir, err := os.MkdirTemp("", "skill-install-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write files to temp directory
	skillDir := filepath.Join(tempDir, bundle.Name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	for relPath, content := range bundle.Files {
		filePath := filepath.Join(skillDir, relPath)
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", relPath, err)
		}
	}

	// Use existing SkillUploader to upload the skill
	uploader := NewSkillUploader(c.client, c.fileProvider)
	uploader.SetSkillProvider(c.skillProvider)
	uploader.SetForce(force)

	version := bundle.Metadata.Version
	if version == "" {
		version = "1.0.0"
	}

	return uploader.UploadSkill(ctx, skillDir, version, fmt.Sprintf("skills/%s", spaceID), bundle.Name)
}

// skillExists checks if a skill already exists
func (c *SkillInstallCommand) skillExists(ctx stdctx.Context, spaceID, skillName string) (bool, error) {
	folderPath := fmt.Sprintf("skills/%s/%s", spaceID, skillName)
	_, err := c.fileProvider.List(ctx, folderPath, nil)
	if err != nil {
		// If error, likely doesn't exist
		return false, nil
	}
	return true, nil
}

// updateIndex updates the search index for a skill
// Note: Indexing is now handled by SkillUploader during upload
func (c *SkillInstallCommand) updateIndex(ctx stdctx.Context, spaceID, skillName string) error {
	// Indexing is automatically performed by SkillUploader.UploadSkill
	// This method is kept for potential future use
	return nil
}

// parseArgs parses command arguments
func (c *SkillInstallCommand) parseArgs(args []string) (*InstallSkillArgs, error) {
	result := &InstallSkillArgs{}

	var nonFlagArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch arg {
		case "-h", "--help":
			result.ShowHelp = true
			return result, nil
		case "-v", "--version":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				result.Version = args[i+1]
				i++
			} else {
				return nil, fmt.Errorf("version flag requires a value")
			}
		case "-n", "--name":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				result.SkillName = args[i+1]
				i++
			} else {
				return nil, fmt.Errorf("name flag requires a value")
			}
		case "-f", "--force":
			result.Force = true
		case "--skip-verify":
			result.SkipVerify = true
		default:
			if !strings.HasPrefix(arg, "-") {
				nonFlagArgs = append(nonFlagArgs, arg)
			}
		}
	}

	// Parse space and source ref
	if len(nonFlagArgs) < 1 {
		return nil, fmt.Errorf("space ID is required")
	}
	if len(nonFlagArgs) < 2 {
		return nil, fmt.Errorf("source reference is required (local path or remote identifier)")
	}

	result.SpaceID = nonFlagArgs[0]
	result.SourceRef = nonFlagArgs[1]

	return result, nil
}

// PrintHelp prints the help message
func (c *SkillInstallCommand) PrintHelp() {
	fmt.Println(`Usage: install-skill <space> <source> [options]

Install a skill from multiple sources into a RAGFlow space.

Arguments:
  <space>                  Target skills space ID (required)
  <source>                 Skill source reference (required):
                           - Local: ./path/to/skill or /absolute/path
                           - GitHub: github.com/owner/repo/path/to/skill
                           - ClawHub: clawhub://owner/skill-name or clawhub.ai/owner/skill-name
                           - skills.sh: skill://skill-name or skills.sh/skill/name

Options:
  -v, --version string     Specify skill version (default: from SKILL.md or 1.0.0)
  -n, --name string        Override skill name (default: from SKILL.md)
  -f, --force              Force reinstall if skill exists (deletes existing first)
  --skip-verify            Skip security verification (use with caution)
  -h, --help               Show this help message

Security:
  By default, all skills are scanned for potential security threats before
  installation. The scan checks for:
    - Data exfiltration patterns (curl $SECRET, .ssh access, etc.)
    - Prompt injection attempts (DAN mode, ignore instructions, etc.)
    - Destructive commands (rm -rf /, mkfs, etc.)
    - Persistence mechanisms (cron, .bashrc, authorized_keys, etc.)
    - Network threats (reverse shells, tunneling, etc.)
    - Obfuscation (base64 | bash, eval(), etc.)

  Trust levels:
    - builtin:   Official RAGFlow skills (always allowed)
    - trusted:   openai/skills, anthropics/skills (caution allowed)
    - community: All other sources (findings blocked unless --force)

Examples:
  # Install from local path
  install-skill my-space ./my-local-skill

  # Install from GitHub
  install-skill my-space github.com/openai/skills/skill-creator

  # Force reinstall (delete existing and reinstall)
  install-skill my-space ./my-skill --force

  # Force install with custom name, skip security check
  install-skill my-space claw://unknown-skill --force --name my-skill --skip-verify

  # Install specific version
  install-skill my-space skill://kubernetes --version 2.1.0

Note: 'add-skill' command is deprecated. Use 'install-skill' instead.`)
}

// getDir extracts directory from file path
func getDir(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx == -1 {
		return ""
	}
	return path[:idx]
}
