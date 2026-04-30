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
	"strings"
)

// UninstallSkillArgs holds the parsed arguments for uninstall-skill command
type UninstallSkillArgs struct {
	SkillName string
	SpaceID   string
	ShowHelp  bool
}

// SkillUninstallCommand handles the uninstall-skill command
type SkillUninstallCommand struct {
	client        HTTPClientInterface
	skillProvider Provider
	fileProvider  *FileProvider
}

// NewUninstallSkillCommand creates a new uninstall-skill command handler
func NewUninstallSkillCommand(client HTTPClientInterface, skillProvider Provider, fileProvider *FileProvider) *SkillUninstallCommand {
	return &SkillUninstallCommand{
		client:        client,
		skillProvider: skillProvider,
		fileProvider:  fileProvider,
	}
}

// Execute runs the uninstall-skill command
func (c *SkillUninstallCommand) Execute(args []string) error {
	parsedArgs, err := c.parseArgs(args)
	if err != nil {
		return err
	}

	if parsedArgs.ShowHelp {
		c.PrintHelp()
		return nil
	}

	return c.uninstallSkill(stdctx.Background(), parsedArgs.SpaceID, parsedArgs.SkillName)
}

// uninstallSkill deletes a skill and its index
func (c *SkillUninstallCommand) uninstallSkill(ctx stdctx.Context, spaceID, skillName string) error {
	if c.skillProvider == nil {
		return fmt.Errorf("skill provider not available")
	}

	fmt.Printf("Uninstalling skill '%s' from space '%s'...\n\n", skillName, spaceID)

	var indexErr, folderErr error

	// 1. Delete search index
	skillProvider, ok := c.skillProvider.(*SkillProvider)
	if ok {
		fmt.Printf("Deleting search index for skill '%s'...\n", skillName)
		if err := skillProvider.DeleteSkill(ctx, spaceID, skillName); err != nil {
			indexErr = fmt.Errorf("failed to delete search index: %w", err)
			fmt.Printf("⚠ %v\n", indexErr)
		} else {
			fmt.Printf("✓ Search index deleted\n")
		}
	}

	// 2. Delete file system folder
	if c.fileProvider != nil {
		fmt.Printf("Deleting skill folder '%s/%s'...\n", spaceID, skillName)
		folderPath := fmt.Sprintf("skills/%s/%s", spaceID, skillName)
		if err := c.fileProvider.DeleteFolderByPath(ctx, folderPath); err != nil {
			folderErr = fmt.Errorf("failed to delete skill folder: %w", err)
			fmt.Printf("⚠ %v\n", folderErr)
		} else {
			fmt.Printf("✓ Skill folder deleted\n")
		}
	}

	// 3. Report results
	fmt.Println()

	if indexErr != nil && folderErr != nil {
		return fmt.Errorf("failed to completely uninstall skill '%s': index deletion failed (%v), folder deletion failed (%v)",
			skillName, indexErr, folderErr)
	}
	if indexErr != nil {
		return fmt.Errorf("failed to uninstall skill '%s': %w", skillName, indexErr)
	}
	if folderErr != nil {
		return fmt.Errorf("failed to uninstall skill '%s': %w", skillName, folderErr)
	}

	fmt.Printf("✓ Successfully uninstalled skill '%s'\n", skillName)
	return nil
}

// parseArgs parses command arguments
func (c *SkillUninstallCommand) parseArgs(args []string) (*UninstallSkillArgs, error) {
	result := &UninstallSkillArgs{}

	var nonFlagArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch arg {
		case "-h", "--help":
			result.ShowHelp = true
			return result, nil
		default:
			if !strings.HasPrefix(arg, "-") {
				nonFlagArgs = append(nonFlagArgs, arg)
			}
		}
	}

	// Parse space and skill name
	if len(nonFlagArgs) < 1 {
		return nil, fmt.Errorf("space ID is required")
	}
	if len(nonFlagArgs) < 2 {
		return nil, fmt.Errorf("skill name is required")
	}

	result.SpaceID = nonFlagArgs[0]
	result.SkillName = nonFlagArgs[1]

	return result, nil
}

// PrintHelp prints the help message
func (c *SkillUninstallCommand) PrintHelp() {
	fmt.Println(`Usage: uninstall-skill <space> <skill-name>

Remove a skill from RAGFlow and delete its search index.

Arguments:
  <space>                  Skills space ID (required)
  <skill-name>             Name of the skill to uninstall (required)

Options:
  -h, --help               Show this help message

Examples:
  uninstall-skill my-space my-skill
  uninstall-skill production document-analyzer

Note: 'delete-skill' command is deprecated. Use 'uninstall-skill' instead.`)
}
