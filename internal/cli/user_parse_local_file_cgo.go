//go:build cgo

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
	"os"

	"ragflow/internal/ingestion/parser"
	"ragflow/internal/utility"
)

func (c *CLI) UserParseLocalFile(cmd *Command) (ResponseIf, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	filename, ok := cmd.Params["filename"].(string)
	if !ok {
		return nil, fmt.Errorf("filename not provided")
	}
	visionModel, ok := cmd.Params["vision_model"].(string)
	if !ok {
		visionModel = ""
	}
	chatModel, ok := cmd.Params["chat_model"].(string)
	if !ok {
		chatModel = ""
	}
	asrModel, ok := cmd.Params["asr_model"].(string)
	if !ok {
		asrModel = ""
	}
	ocrModel, ok := cmd.Params["ocr_model"].(string)
	if !ok {
		ocrModel = ""
	}
	embeddingModel, ok := cmd.Params["embedding_model"].(string)
	if !ok {
		embeddingModel = ""
	}
	docParseModel, ok := cmd.Params["doc_parse_model"].(string)
	if !ok {
		docParseModel = ""
	}

	fileType := utility.GetFileType(filename)
	config := map[string]string{
		"lib_type": "office_oxide",
	}
	fileParser, err := parser.GetParser(fileType, config)
	if err != nil {
		return nil, err
	}

	fileContent, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read dsl file: %w", err)
	}

	if err = fileParser.Parse(filename, fileContent); err != nil {
		return nil, formatRequestError("parse local file", err)
	}

	var result SimpleResponse
	result.Code = 0
	result.Message = fmt.Sprintf("Success to parse local file %q, vision: %v, chat: %v, asr: %v, ocr: %v, embedding: %v, doc_parse: %v", filename, visionModel, chatModel, asrModel, ocrModel, embeddingModel, docParseModel)
	fmt.Println(result.Message)
	return &result, nil
}
