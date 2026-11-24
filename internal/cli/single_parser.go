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
	"fmt"
	"strings"
)

func (p *Parser) parseSingleCommand() (*Command, error) {
	switch p.curToken.Type {
	case TokenLS, TokenCat, TokenSearch:
		return p.parseFileSystemCommands()
	case TokenWS:
		return p.parseSingleWorkspaceCommand()
	default:
		return nil, fmt.Errorf("unknown command: %s", p.curToken.Value)
	}
}

func (p *Parser) parseSingleWorkspaceCommand() (*Command, error) {
	p.nextToken() // consume WS

	switch p.curToken.Type {
	case TokenList, TokenLS:
		return p.parseSingleListFiles()
	case TokenCat:
		return p.parseSingleCatFile()
	case TokenSearch:
		return p.parseSingleSearch()
	case TokenFind:
		return p.parseSingleFind()
	case TokenUse:
		return p.parseSingleUseWorkspace()
	case TokenNew:
		return p.parseSingleNewWorkspace()
	case TokenDelete:
		return p.parseSingleDeleteWorkspace()
	case TokenSave:
		return p.parseSingleSaveMessage()
	case TokenEOF, TokenSemicolon:
		return p.parseSingleShowCurrentWorkspace()
	default:
		return nil, fmt.Errorf("unknown command: %s", p.curToken.Value)
	}
}

func (p *Parser) parseSingleListFiles() (*Command, error) {
	p.nextToken() // consume COMMAND

	cmd := NewCommand("single_list_files")
	cmd.Params["command"] = p.original

	return cmd, nil
}

func (p *Parser) parseSingleCatFile() (*Command, error) {
	p.nextToken() // consume COMMAND

	cmd := NewCommand("single_cat_file")
	cmd.Params["command"] = p.original

	return cmd, nil
}

func (p *Parser) parseSingleSearch() (*Command, error) {
	p.nextToken() // consume COMMAND

	cmd := NewCommand("single_search")
	cmd.Params["command"] = p.original

	return cmd, nil
}

func (p *Parser) parseSingleFind() (*Command, error) {
	p.nextToken() // consume COMMAND

	cmd := NewCommand("single_find")
	cmd.Params["command"] = p.original

	return cmd, nil
}

func (p *Parser) parseSingleUseWorkspace() (*Command, error) {
	p.nextToken() // consume COMMAND

	cmd := NewCommand("single_use_workspace")
	cmd.Params["command"] = p.original

	workspace, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	cmd.Params["workspace"] = workspace

	return cmd, nil
}

func (p *Parser) parseSingleNewWorkspace() (*Command, error) {
	p.nextToken() // consume COMMAND

	cmd := NewCommand("single_new_workspace")
	cmd.Params["command"] = p.original

	newWorkspace, err := p.parseIdentifier()
	if err != nil {
		fmt.Println(p.curToken.Type)
		return nil, err
	}

	cmd.Params["new_workspace"] = newWorkspace

	return cmd, nil
}

func (p *Parser) parseSingleDeleteWorkspace() (*Command, error) {
	p.nextToken() // consume COMMAND

	cmd := NewCommand("single_delete_workspace")
	cmd.Params["command"] = p.original

	workspace, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	cmd.Params["workspace"] = workspace

	return cmd, nil
}

func (p *Parser) parseSingleSaveMessage() (*Command, error) {
	p.nextToken() // consume COMMAND

	cmd := NewCommand("single_save_message")
	cmd.Params["command"] = p.original

	message, err := messageAfterCommandWords(p.original, "ws", "save")
	if err != nil {
		return nil, err
	}
	cmd.Params["message"] = message

	return cmd, nil
}

func messageAfterCommandWords(original string, words ...string) (string, error) {
	rest := original
	for _, word := range words {
		rest = strings.TrimLeft(rest, " \t")
		end := strings.IndexAny(rest, " \t")
		if end < 0 {
			return "", fmt.Errorf("invalid command %q: missing %s", original, word)
		}
		if !strings.EqualFold(rest[:end], word) {
			return "", fmt.Errorf("invalid command %q: expected %s, got %s", original, word, rest[:end])
		}
		rest = rest[end:]
	}

	message := strings.TrimLeft(rest, " \t")
	if message == "" {
		return "", fmt.Errorf("missing message in %q", original)
	}
	return message, nil
}

func (p *Parser) parseSingleShowCurrentWorkspace() (*Command, error) {
	p.nextToken() // consume COMMAND

	cmd := NewCommand("single_show_current_workspace")
	cmd.Params["command"] = p.original

	return cmd, nil
}
