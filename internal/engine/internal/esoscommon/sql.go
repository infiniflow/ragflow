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

package esoscommon

import (
	"fmt"
	"regexp"
	"strings"

	"ragflow/internal/tokenizer"
)

var preprocessWhitespaceRe = regexp.MustCompile("[ `]+")
var lktksMatchRe = regexp.MustCompile(` ([a-z_]+_l?tks)( like | ?= ?)'([^']+)'`)

// PreprocessSQL normalizes SQL for ES: collapses whitespace/backticks,
// strips '%', and rewrites `<field>_l?tks like/= 'value'` into a
// tokenized MATCH() call.
func PreprocessSQL(sql string) string {
	sql = preprocessWhitespaceRe.ReplaceAllString(sql, " ")
	sql = strings.ReplaceAll(sql, "%", "")

	// Collect replacements so we don't re-scan tokens we've already rewritten
	type replacement struct {
		old, new string
	}
	var replaces []replacement
	for _, m := range lktksMatchRe.FindAllStringSubmatchIndex(sql, -1) {
		match := sql[m[0]:m[1]]
		fld := sql[m[2]:m[3]]
		val := sql[m[6]:m[7]]
		tokenized, err := tokenizer.Tokenize(val)
		if err != nil {
			continue
		}
		fine, err := tokenizer.FineGrainedTokenize(tokenized)
		if err != nil {
			continue
		}
		replaces = append(replaces, replacement{
			old: match,
			// fine comes from tokenizer.FineGrainedTokenize, which strips
			// non-alphanumerics; defense-in-depth escape just in case a
			// future tokenizer change reintroduces a quote.
			new: fmt.Sprintf(" MATCH(%s, '%s', 'operator=OR;minimum_should_match=30%%') ", fld, strings.ReplaceAll(fine, "'", "''")),
		})
	}
	for _, r := range replaces {
		sql = strings.Replace(sql, r.old, r.new, 1)
	}
	return sql
}
