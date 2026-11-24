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

import "fmt"

func (c *CLI) SingleListFilesCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	result := SimpleResponse{}
	result.Code = 1
	result.SetOutputFormat(c.Config.OutputFormat)
	result.Message = "List files not implemented"
	return &result, nil
}

func (c *CLI) SingleCatFileCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	result := SimpleResponse{}
	result.Code = 1
	result.SetOutputFormat(c.Config.OutputFormat)
	result.Message = "Cat file not implemented"
	return &result, nil
}

func (c *CLI) SingleSearchCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	result := SimpleResponse{}
	result.Code = 1
	result.SetOutputFormat(c.Config.OutputFormat)
	result.Message = "Search not implemented"
	return &result, nil
}

func (c *CLI) SingleFindCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	result := SimpleResponse{}
	result.Code = 1
	result.SetOutputFormat(c.Config.OutputFormat)
	result.Message = "Find not implemented"
	return &result, nil
}

func (c *CLI) SingleUseWorkspaceCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	result := SimpleResponse{}
	result.Code = 0
	result.SetOutputFormat(c.Config.OutputFormat)
	result.Message = fmt.Sprintf("Use workspace %s", cmd.Params["workspace"])
	return &result, nil
}

func (c *CLI) SingleNewWorkspaceCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	result := SimpleResponse{}
	result.Code = 1
	result.SetOutputFormat(c.Config.OutputFormat)
	result.Message = fmt.Sprintf("New workspace %s", cmd.Params["new_workspace"])
	return &result, nil
}

func (c *CLI) SingleDeleteWorkspaceCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	result := SimpleResponse{}
	result.Code = 1
	result.SetOutputFormat(c.Config.OutputFormat)
	result.Message = fmt.Sprintf("Delete workspace %s", cmd.Params["workspace"])
	return &result, nil
}

func (c *CLI) SingleSaveMessageCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	result := SimpleResponse{}
	result.Code = 1
	result.SetOutputFormat(c.Config.OutputFormat)
	result.Message = fmt.Sprintf("Attempt to save message: %s", cmd.Params["message"])
	return &result, nil
}

func (c *CLI) SingleShowCurrentWorkspaceCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	result := SimpleResponse{}
	result.Code = 1
	result.SetOutputFormat(c.Config.OutputFormat)
	result.Message = "Show current workspace not implemented"
	return &result, nil
}
