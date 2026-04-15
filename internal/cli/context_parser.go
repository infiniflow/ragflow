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

func (p *Parser) parseContextListCommand() (*Command, error) {
	p.nextToken() // consume LS

	cmd := NewCommand("context_list")

	if p.curToken.Type == TokenEOF {
		cmd.Params["path"] = "."
		return cmd, nil
	}

	for p.curToken.Type != TokenEOF {
		if p.curToken.Type == TokenDash {
			p.nextToken() // skip dash
			if p.curToken.Type != TokenIdentifier {
				return nil, fmt.Errorf("expect identifier")
			}
			if cmd.Params["parameter"] == nil {
				cmd.Params["parameter"] = p.curToken.Value
			} else {
				cmd.Params["parameter"] = fmt.Sprintf("%s%s", cmd.Params["parameter"], p.curToken.Value)
			}
			p.nextToken() // skip parameter
		} else if p.curToken.Type == TokenIdentifier {
			if cmd.Params["path"] == nil {
				cmd.Params["path"] = p.curToken.Value
			} else {
				err := fmt.Errorf("ls: cannot access '%s': No such file or directory", p.curToken.Value)
				return nil, err
			}
			p.nextToken() // skip path
		} else {
			return nil, fmt.Errorf("syntax error")
		}
	}

	return cmd, nil
}

func (p *Parser) parseContextCatCommand() (*Command, error) {
	p.nextToken() // consume CAT

	if p.curToken.Type == TokenEOF {
		return nil, fmt.Errorf("expect a filename")
	}

	if p.curToken.Type != TokenIdentifier && p.curToken.Type != TokenQuotedString {
		return nil, fmt.Errorf("expect a filename")
	}

	cmd := NewCommand("context_cat")
	if p.curToken.Type == TokenIdentifier {
		for p.curToken.Type != TokenEOF {
			if p.curToken.Type != TokenIdentifier {
				return nil, fmt.Errorf("expect a identifier")
			}

			if cmd.Params["filename"] == nil {
				cmd.Params["filename"] = p.curToken.Value
			} else {
				cmd.Params["filename"] = fmt.Sprintf("%s/%s", cmd.Params["filename"], p.curToken.Value)
			}
			p.nextToken()
			if p.curToken.Type == TokenEOF {
				break
			}
			if p.curToken.Type != TokenSlash {
				return nil, fmt.Errorf("expect a slash")
			}
			p.nextToken()
			if p.curToken.Type == TokenEOF {
				return nil, fmt.Errorf("error format")
			}
		}

	} else if p.curToken.Type == TokenQuotedString {
		var err error
		cmd.Params["filename"], err = p.parseQuotedString()
		if err != nil {
			return nil, err
		}
	}
	p.nextToken()

	if p.curToken.Type != TokenEOF {
		return nil, fmt.Errorf("syntax error")
	}

	return cmd, nil
}

func (p *Parser) parseContextSearchCommand() (*Command, error) {
	p.nextToken() // consume SEARCH

	cmd := NewCommand("context_search")

	for p.curToken.Type != TokenEOF {
		if p.curToken.Type == TokenDash {
			p.nextToken() // skip dash
			if p.curToken.Type != TokenIdentifier {
				return nil, fmt.Errorf("expect identifier")
			}

			if strings.ToLower(p.curToken.Value) == "n" {
				p.nextToken()
				var err error
				if p.curToken.Type != TokenInteger {
					return nil, fmt.Errorf("expect number")
				}
				cmd.Params["number"], err = p.parseNumber()
				if err != nil {
					return nil, err
				}
				p.nextToken()
				continue
			}

			if strings.ToLower(p.curToken.Value) == "t" {
				p.nextToken()
				var err error
				if p.curToken.Type != TokenInteger {
					return nil, fmt.Errorf("expect number")
				}
				cmd.Params["threshold"], err = p.parseFloat()
				if err != nil {
					return nil, err
				}
				p.nextToken()
				continue
			}

			return nil, fmt.Errorf("unknow parameter: %s", p.curToken.Value)
		} else if p.curToken.Type == TokenIdentifier {
			if cmd.Params["path"] == nil {
				cmd.Params["path"] = p.curToken.Value
			} else {
				cmd.Params["path"] = fmt.Sprintf("%s %s", cmd.Params["path"], p.curToken.Value)
			}
			p.nextToken() // skip path
		} else if p.curToken.Type == TokenQuotedString {
			if cmd.Params["query"] == nil {
				var err error
				cmd.Params["query"], err = p.parseQuotedString()
				if err != nil {
					return nil, err
				}
				p.nextToken()
			} else {
				return nil, fmt.Errorf("Query phrase exists")
			}
		}
		return nil, fmt.Errorf("syntax error")
	}

	return cmd, nil
}
