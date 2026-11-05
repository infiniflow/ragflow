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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// LIST USERS WITH keyword 'alice' status 'active' order 'create_time' sort 'asc'
// page 1 page_size 10 role 'Reader' plan 'Pro' quota 80 days 30;
func (p *Parser) parseAdminUserListOptions() (*Command, error) {
	p.nextToken() // WITH
	cmd := NewCommand("admin_list_users_command")
	for p.curToken.Type != TokenEOF && p.curToken.Type != TokenSemicolon {
		name := strings.ToLower(p.curToken.Value)
		if !isAdminUserListParameter(name) {
			return nil, fmt.Errorf("unknown LIST USERS parameter %q", name)
		}
		if _, exists := cmd.Params[name]; exists {
			return nil, fmt.Errorf("duplicate LIST USERS parameter %q", name)
		}
		p.nextToken()
		var value string
		if name == "page" || name == "page_size" || name == "quota" || name == "top" || name == "days" {
			number, err := p.parseNumber()
			if err != nil {
				return nil, fmt.Errorf("invalid %s: %w", name, err)
			}
			if number < 0 || ((name == "page" || name == "page_size" || name == "top") && number == 0) || (name == "quota" && number > 100) {
				return nil, fmt.Errorf("invalid %s value %d", name, number)
			}
			value = strconv.Itoa(number)
		} else {
			var err error
			value, err = p.parseQuotedString()
			if err != nil {
				return nil, fmt.Errorf("invalid %s: %w", name, err)
			}
		}
		cmd.Params[name] = value
		p.nextToken()
	}
	if len(cmd.Params) == 0 {
		return nil, fmt.Errorf("expected a LIST USERS parameter after WITH")
	}
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	if p.curToken.Type != TokenEOF {
		return nil, fmt.Errorf("unexpected token after LIST USERS: %s", p.curToken.Value)
	}
	if err := validateAdminUserListMode(cmd.Params); err != nil {
		return nil, err
	}
	return cmd, nil
}

func validateAdminUserListMode(params map[string]interface{}) error {
	if _, top := params["top"]; top {
		for _, name := range []string{"page", "page_size"} {
			if _, present := params[name]; present {
				return fmt.Errorf("top cannot be combined with page or page_size")
			}
		}
		field, ok := params["order_by"]
		if !ok {
			field = params["order"]
		}
		switch strings.ToLower(fmt.Sprint(field)) {
		case "storage", "documents", "index":
		default:
			return fmt.Errorf("top is only supported for storage, documents and index")
		}
	}
	return nil
}

func isAdminUserListParameter(name string) bool {
	switch name {
	case "keyword", "status", "order", "sort", "page", "page_size", "role", "plan", "quota", "top", "days":
		return true
	default:
		return false
	}
}

func (c *CLI) AdminListUsersCommand(commandCount int, cmd *Command) (ResponseIf, error) {
	if c.Config == nil || c.Config.CLIMode != AdminMode || c.AdminServerClient == nil || c.AdminServerClient.LoginToken == nil {
		return nil, fmt.Errorf("listing users requires admin mode and login")
	}
	query := url.Values{}
	for name, value := range cmd.Params {
		if !isAdminUserListParameter(name) {
			return nil, fmt.Errorf("unknown LIST USERS parameter %q", name)
		}
		query.Set(name, fmt.Sprint(value))
	}
	path := "/admin/users"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	response, err := c.AdminServerClient.Request(commandCount, http.MethodGet, path, "admin", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list users: HTTP %d, body: %s", response.StatusCode, response.Body)
	}
	var result adminUserListResponse
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return nil, fmt.Errorf("list users failed: invalid JSON (%w)", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("%s", result.Message)
	}
	result.Duration = response.Duration
	result.SetOutputFormat(c.Config.OutputFormat)
	return &result, nil
}

type adminUserListResponse struct {
	CommonResponse
	Total int64 `json:"total"`
}

func (r *adminUserListResponse) PrintOut() {
	if r.OutputFormat == OutputFormatJSON {
		data, err := json.MarshalIndent(struct {
			Users []map[string]interface{} `json:"users"`
			Total int64                    `json:"total"`
		}{Users: r.Data, Total: r.Total}, "", "  ")
		if err != nil {
			fmt.Printf("Error marshaling JSON: %v\n", err)
			return
		}
		fmt.Println(string(data))
		return
	}
	r.CommonResponse.PrintOut()
	fmt.Printf("Total: %d\n", r.Total)
}
