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

//go:build !ee

// Admin ingestion and per-user inspection commands are enterprise features.
// The OSS build keeps the parser and dispatcher identical and rejects those
// commands at runtime.
package cli

import "errors"

func (c *CLI) AdminListUserAgentsCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	return nil, errors.New("listing user agents is only available in the enterprise edition")
}

func (p *Parser) parseAdminListUserAgentsEE(userName string) (*Command, error) {
	return nil, errors.New("listing user agents is only available in the enterprise edition")
}

func (c *CLI) AdminListUserDatasetsCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	return nil, errors.New("listing user datasets is only available in the enterprise edition")
}

func (p *Parser) parseAdminListUserDatasetsEE(userName string) (*Command, error) {
	return nil, errors.New("listing user datasets is only available in the enterprise edition")
}

func (c *CLI) ListAdminIngestionTasks(commandCount int, cmd *Command) (ResponseIf, error) {
	return nil, errors.New("listing admin ingestion tasks is only available in the enterprise edition")
}

func (p *Parser) parseAdminListIngestionTasks() (*Command, error) {
	return nil, errors.New("listing admin ingestion tasks is only available in the enterprise edition")
}

func (p *Parser) parseAdminShowIngestionLog() (*Command, error) {
	return nil, errors.New("ingestion logs are only available in the enterprise edition")
}

func (c *CLI) AdminShowIngestionLogCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	return nil, errors.New("ingestion logs are only available in the enterprise edition")
}

func (c *CLI) AdminShowIngestionMessagesCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	return nil, errors.New("ingestion logs are only available in the enterprise edition")
}

func (c *CLI) AdminListUserIngestionTasksCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	return nil, errors.New("listing user ingestion tasks is only available in the enterprise edition")
}

func (p *Parser) parseAdminListUserIngestionTasks(userName string) (*Command, error) {
	return nil, errors.New("listing user ingestion tasks is only available in the enterprise edition")
}

func (c *CLI) AdminStopIngestionCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	return nil, errors.New("stopping admin ingestion tasks is only available in the enterprise edition")
}

func (c *CLI) AdminRemoveIngestionCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	return nil, errors.New("removing admin ingestion tasks is only available in the enterprise edition")
}

func (c *CLI) AdminStopUserIngestionTasksCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	return nil, errors.New("stopping user ingestion tasks is only available in the enterprise edition")
}

func (c *CLI) AdminRemoveUserIngestionTasksCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	return nil, errors.New("removing user ingestion tasks is only available in the enterprise edition")
}

func (p *Parser) parseAdminStopIngestionTasks() (*Command, error) {
	return nil, errors.New("stopping admin ingestion tasks is only available in the enterprise edition")
}

func (p *Parser) parseAdminRemoveIngestionTasks() (*Command, error) {
	return nil, errors.New("removing admin ingestion tasks is only available in the enterprise edition")
}

func (p *Parser) parseAdminStopUserIngestionTasks(userName string) (*Command, error) {
	return nil, errors.New("stopping user ingestion tasks is only available in the enterprise edition")
}

func (p *Parser) parseAdminRemoveUserIngestionTasks(userName string) (*Command, error) {
	return nil, errors.New("removing user ingestion tasks is only available in the enterprise edition")
}
