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

import "errors"

// AdminListUserAgentsCommand is available only in the enterprise edition.
func (c *CLI) AdminListUserAgentsCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	return nil, errors.New("listing user agents is only available in the enterprise edition")
}

func (p *Parser) parseAdminListUserAgentsEE(userName string) (*Command, error) {
	return nil, errors.New("listing user agents is only available in the enterprise edition")
}

// AdminListUserDatasetsCommand is available only in the enterprise edition.
func (c *CLI) AdminListUserDatasetsCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	return nil, errors.New("listing user datasets is only available in the enterprise edition")
}

func (p *Parser) parseAdminListUserDatasetsEE(userName string) (*Command, error) {
	return nil, errors.New("listing user datasets is only available in the enterprise edition")
}
