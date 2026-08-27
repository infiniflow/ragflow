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
	"strconv"
)

// INSERT CHUNKS FROM FILE "file_path"
// INSERT METADATA FROM FILE "file_path"
func (p *Parser) parseDevInsertCommand() (*Command, error) {
	p.nextToken() // consume INSERT

	// Expect CHUNKS or METADATA
	if p.curToken.Type == TokenChunks {
		return p.parseInsertChunksFromFile()
	}
	if p.curToken.Type == TokenMetadata {
		return p.parseInsertMetadataFromFile()
	}
	return nil, fmt.Errorf("expected CHUNKS or METADATA after INSERT, got %s", p.curToken.Value)
}

// Internal CLI for GO
// parseInsertChunksFromFile parses: INSERT CHUNKS FROM FILE "file_path"
func (p *Parser) parseInsertChunksFromFile() (*Command, error) {
	p.nextToken() // consume CHUNKS

	// Expect FROM
	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM, got %s", p.curToken.Value)
	}
	p.nextToken()

	// Expect FILE
	if p.curToken.Type != TokenFile {
		return nil, fmt.Errorf("expected FILE, got %s", p.curToken.Value)
	}
	p.nextToken()

	// Get file path (quoted string)
	filePath, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("dev_insert_chunks_from_file")
	cmd.Params["file_path"] = filePath

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// Internal CLI for GO
// parseInsertMetadataFromFile parses: INSERT METADATA FROM FILE "file_path"
func (p *Parser) parseInsertMetadataFromFile() (*Command, error) {
	p.nextToken() // consume METADATA

	// Expect FROM
	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM, got %s", p.curToken.Value)
	}
	p.nextToken()

	// Expect FILE
	if p.curToken.Type != TokenFile {
		return nil, fmt.Errorf("expected FILE, got %s", p.curToken.Value)
	}
	p.nextToken()

	// Get file path (quoted string)
	filePath, err := p.parseQuotedString()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("dev_insert_metadata_from_file")
	cmd.Params["file_path"] = filePath

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

// parseDevGetCommand parses: GET CHUNK or GET METADATA
func (p *Parser) parseDevGetCommand() (*Command, error) {
	p.nextToken() // consume GET

	if p.curToken.Type == TokenChunk {
		return p.parseGetChunk()
	}
	if p.curToken.Type == TokenMetadata {
		return p.parseGetMetadata()
	}

	return nil, fmt.Errorf("unknown GET target: %s", p.curToken.Value)
}

// parseGetChunk parses: GET CHUNK 'chunk_id' OF DOCUMENT 'doc_id' IN DATASET 'dataset_id'
func (p *Parser) parseGetChunk() (*Command, error) {
	p.nextToken() // consume CHUNK

	// Parse chunk_id
	chunkID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected chunk_id: %w", err)
	}

	cmd := NewCommand("dev_get_chunk")
	cmd.Params["chunk_id"] = chunkID

	p.nextToken()
	if p.curToken.Type != TokenOf {
		return nil, fmt.Errorf("expected OF after chunk_id")
	}
	p.nextToken()

	if p.curToken.Type != TokenDocument {
		return nil, fmt.Errorf("expected DOCUMENT after OF")
	}
	p.nextToken()

	// Parse doc_id
	docID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected doc_id: %w", err)
	}
	cmd.Params["doc_id"] = docID

	p.nextToken()
	if p.curToken.Type != TokenIn {
		return nil, fmt.Errorf("expected IN after doc_id")
	}
	p.nextToken()

	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected DATASET after IN")
	}
	p.nextToken()

	// Parse dataset_id
	datasetID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected dataset_id: %w", err)
	}
	cmd.Params["dataset_id"] = datasetID

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// Internal
// parseDevUpdateCommand parses: UPDATE CHUNK 'chunk_id' OF DATASET 'dataset_name' SET '{"content": "..."}'
func (p *Parser) parseDevUpdateCommand() (*Command, error) {
	p.nextToken() // consume UPDATE

	if p.curToken.Type == TokenChunk {
		return p.parseUpdateChunk()
	}

	return nil, fmt.Errorf("unknown UPDATE target: %s", p.curToken.Value)
}

// Internal CLI for GO
// parseUpdateChunk parses: UPDATE CHUNK 'chunk_id' OF DOCUMENT 'doc_id' IN DATASET 'dataset_id' SET '{"content": "..."}'
func (p *Parser) parseUpdateChunk() (*Command, error) {
	p.nextToken() // consume CHUNK

	// Parse chunk_id
	chunkID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected chunk_id: %w", err)
	}

	cmd := NewCommand("dev_update_chunk")
	cmd.Params["chunk_id"] = chunkID

	p.nextToken()
	if p.curToken.Type != TokenOf {
		return nil, fmt.Errorf("expected OF after chunk_id")
	}
	p.nextToken()

	if p.curToken.Type != TokenDocument {
		return nil, fmt.Errorf("expected DOCUMENT after OF")
	}
	p.nextToken()

	// Parse doc_id
	docID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected doc_id: %w", err)
	}
	cmd.Params["doc_id"] = docID

	p.nextToken()
	if p.curToken.Type != TokenIn {
		return nil, fmt.Errorf("expected IN after doc_id")
	}
	p.nextToken()

	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected DATASET after IN")
	}
	p.nextToken()

	// Parse dataset_name
	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected dataset_name: %w", err)
	}
	cmd.Params["dataset_name"] = datasetName

	p.nextToken()
	if p.curToken.Type != TokenSet {
		return nil, fmt.Errorf("expected SET after dataset_name")
	}
	p.nextToken()

	// Parse JSON body
	jsonBody, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected JSON body: %w", err)
	}
	cmd.Params["json_body"] = jsonBody

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// parseDevSetMeta parses: SET METADATA OF DOCUMENT 'doc_id' TO '{"key": "value"}'
func (p *Parser) parseDevSetMeta() (*Command, error) {
	p.nextToken() // consume METADATA

	// Expect OF
	if p.curToken.Type != TokenOf {
		return nil, fmt.Errorf("expected OF after SET METADATA")
	}
	p.nextToken()

	// Expect DOCUMENT
	if p.curToken.Type != TokenDocument {
		return nil, fmt.Errorf("expected DOCUMENT after SET METADATA OF")
	}
	p.nextToken()

	// Parse doc_id
	docID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected doc_id: %w", err)
	}
	cmd := NewCommand("dev_set_meta")
	cmd.Params["doc_id"] = docID

	p.nextToken()
	// Expect TO
	if p.curToken.Type != TokenTo {
		return nil, fmt.Errorf("expected TO after doc_id")
	}
	p.nextToken()

	// Parse meta JSON
	meta, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected meta JSON: %w", err)
	}
	cmd.Params["meta"] = meta

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// parseDevDeleteMeta parses: DELETE METADATA OF DOCUMENT 'doc_id' [KEYS '["key1", "key2"]']
// If KEYS is not provided, deletes entire document metadata
func (p *Parser) parseDevDeleteMeta() (*Command, error) {
	p.nextToken() // consume METADATA

	// Expect OF
	if p.curToken.Type != TokenOf {
		return nil, fmt.Errorf("expected OF after DELETE METADATA")
	}
	p.nextToken()

	// Expect DOCUMENT
	if p.curToken.Type != TokenDocument {
		return nil, fmt.Errorf("expected DOCUMENT after DELETE METADATA OF")
	}
	p.nextToken()

	// Parse doc_id
	docID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected doc_id: %w", err)
	}
	cmd := NewCommand("dev_delete_meta")
	cmd.Params["doc_id"] = docID

	p.nextToken()
	// KEYS is optional - if not provided, delete entire document metadata
	if p.curToken.Type != TokenKeys {
		if p.curToken.Type == TokenSemicolon {
			p.nextToken()
			return cmd, nil
		}
		if p.curToken.Type == TokenEOF {
			return cmd, nil
		}
		return nil, fmt.Errorf("expected KEYS or end of command after doc_id")
	}

	// Parse keys JSON array
	p.nextToken()
	keys, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected keys JSON array: %w", err)
	}
	cmd.Params["keys"] = keys

	p.nextToken()
	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
		return cmd, nil
	}
	if p.curToken.Type != TokenEOF {
		return nil, fmt.Errorf("expected end of command after KEYS")
	}

	return cmd, nil
}

// parseDevRemoveTags parses: REMOVE TAGS 'tag1', 'tag2' from DATASET 'dataset_name';
func (p *Parser) parseDevRemoveTags() (*Command, error) {
	p.nextToken() // consume TAGS

	// Parse first tag
	tag, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected tag: %w", err)
	}
	tags := []string{tag}

	// Parse additional tags separated by commas
	for {
		p.nextToken()
		if p.curToken.Type == TokenComma {
			p.nextToken()
			tag, err := p.parseQuotedString()
			if err != nil {
				return nil, fmt.Errorf("expected tag after comma: %w", err)
			}
			tags = append(tags, tag)
		} else {
			break
		}
	}
	cmd := NewCommand("dev_rm_tags")
	cmd.Params["tags"] = tags

	// Expect from
	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM after tags")
	}
	p.nextToken()

	// Expect DATASET
	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected DATASET after FROM")
	}
	p.nextToken()

	// Parse dataset_name
	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected dataset_name: %w", err)
	}
	cmd.Params["dataset_name"] = datasetName

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// parseDevRemoveChunk parses:
//   - REMOVE CHUNKS 'chunk_id1', 'chunk_id2' FROM DOCUMENT 'doc_id' IN DATASET 'dataset_name';
//   - REMOVE ALL CHUNKS FROM DOCUMENT 'doc_id' IN DATASET 'dataset_name';
func (p *Parser) parseDevRemoveChunk() (*Command, error) {
	cmd := NewCommand("dev_remove_chunks")

	// Check if ALL CHUNKS - if we came here from TokenAll case, curToken is already ALL
	if p.curToken.Type == TokenAll {
		p.nextToken() // consume ALL
		if p.curToken.Type != TokenChunks {
			return nil, fmt.Errorf("expected CHUNKS after ALL")
		}
		p.nextToken() // consume CHUNKS
		cmd.Params["delete_all"] = true
	} else {
		// curToken is TokenChunks, consume it first
		p.nextToken()
		// Multiple chunks: REMOVE CHUNKS 'id1' 'id2' FROM DOCUMENT 'doc_id' IN DATASET 'dataset_name' (space-separated)
		// Parse first chunk ID
		chunkID, err := p.parseQuotedString()
		if err != nil {
			return nil, fmt.Errorf("expected chunk_id: %w", err)
		}
		chunkIDs := []string{chunkID}

		// Parse additional chunk IDs separated by spaces (each quoted)
		for {
			p.nextToken()
			// Stop if we hit FROM or non-quoted token
			if p.curToken.Type == TokenFrom || p.curToken.Type != TokenQuotedString {
				break
			}
			chunkID, err := p.parseQuotedString()
			if err != nil {
				return nil, fmt.Errorf("expected chunk_id: %w", err)
			}
			chunkIDs = append(chunkIDs, chunkID)
		}
		cmd.Params["chunk_ids"] = chunkIDs
	}

	// Expect FROM
	if p.curToken.Type != TokenFrom {
		return nil, fmt.Errorf("expected FROM after chunk(s)")
	}
	p.nextToken()

	// Expect DOCUMENT
	if p.curToken.Type != TokenDocument {
		return nil, fmt.Errorf("expected DOCUMENT after FROM")
	}
	p.nextToken()

	// Parse doc_id
	docID, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected doc_id: %w", err)
	}
	cmd.Params["doc_id"] = docID

	p.nextToken()

	// Expect IN
	if p.curToken.Type != TokenIn {
		return nil, fmt.Errorf("expected IN after doc_id")
	}
	p.nextToken()

	// Expect DATASET
	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected DATASET after IN")
	}
	p.nextToken()

	// Parse dataset_name (quoted string)
	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected dataset_name: %w", err)
	}
	cmd.Params["dataset_name"] = datasetName

	p.nextToken()

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

// Internal CLI for GO
// parseDevDropChunkStore parses: DROP CHUNK STORE for Dataset 'name'
func (p *Parser) parseDevDropChunkStore() (*Command, error) {
	p.nextToken() // consume CHUNK STORE

	// Expect FOR
	if p.curToken.Type != TokenFor {
		return nil, fmt.Errorf("expected FOR after CHUNK STORE, got %s", p.curToken.Value)
	}
	p.nextToken()

	// Expect Dataset
	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected Dataset after FOR, got %s", p.curToken.Value)
	}
	p.nextToken()

	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected dataset name, got %s", p.curToken.Value)
	}

	p.nextToken()
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("dev_drop_chunk_store")
	cmd.Params["dataset_name"] = datasetName
	return cmd, nil
}

// parseDevDropMetadataStore parses: DROP METADATA STORE
func (p *Parser) parseDevDropMetadataStore() (*Command, error) {
	// DROP METADATA STORE
	p.nextToken() // consume METADATA

	if p.curToken.Type != TokenStore {
		return nil, fmt.Errorf("expected STORE after METADATA, got %s", p.curToken.Value)
	}
	p.nextToken()
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("dev_drop_metadata_store")
	return cmd, nil
}

// Internal CLI for GO
// parseDevCreateChunkStore parses: CREATE CHUNK STORE for Dataset 'name' VECTOR SIZE N
func (p *Parser) parseDevCreateChunkStore() (*Command, error) {
	p.nextToken() // consume CHUNK STORE compound token

	// Expect FOR
	if p.curToken.Type != TokenFor {
		return nil, fmt.Errorf("expected FOR after CHUNK STORE, got %s", p.curToken.Value)
	}
	p.nextToken()

	// Expect Dataset
	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected Dataset after FOR, got %s", p.curToken.Value)
	}
	p.nextToken()

	datasetName, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected dataset name, got %s", p.curToken.Value)
	}

	p.nextToken()
	if p.curToken.Type != TokenVector {
		return nil, fmt.Errorf("expected VECTOR after dataset name, got %s", p.curToken.Value)
	}
	p.nextToken()
	if p.curToken.Type != TokenSize {
		return nil, fmt.Errorf("expected SIZE after VECTOR, got %s", p.curToken.Value)
	}
	p.nextToken()

	if p.curToken.Type != TokenInteger {
		return nil, fmt.Errorf("expected vector size number, got %s", p.curToken.Value)
	}
	vectorSize, err := strconv.Atoi(p.curToken.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid vector size: %s", p.curToken.Value)
	}

	p.nextToken()
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	cmd := NewCommand("dev_create_chunk_store")
	cmd.Params["dataset_name"] = datasetName
	cmd.Params["vector_size"] = vectorSize
	return cmd, nil
}

// Internal CLI for GO
// parseDevCreateMetadataStore parses: CREATE METADATA STORE
func (p *Parser) parseDevCreateMetadataStore() (*Command, error) {
	// CREATE METADATA STORE
	p.nextToken() // consume METADATA

	if p.curToken.Type != TokenStore {
		return nil, fmt.Errorf("expected STORE after METADATA, got %s", p.curToken.Value)
	}
	p.nextToken()

	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return NewCommand("dev_create_metadata_store"), nil
}

func (p *Parser) parseCreateRole() (*Command, error) {
	p.nextToken() // consume ROLE
	roleName, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	cmd := NewCommand("create_role")
	cmd.Params["role_name"] = roleName

	p.nextToken()
	if p.curToken.Type == TokenDescription {
		p.nextToken()
		description, err := p.parseQuotedString()
		if err != nil {
			return nil, err
		}
		cmd.Params["description"] = description
		p.nextToken()
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	return cmd, nil
}

func (p *Parser) parseGetMetadata() (*Command, error) {
	p.nextToken() // consume METADATA

	if p.curToken.Type != TokenOf {
		return nil, fmt.Errorf("expected OF after METADATA")
	}
	p.nextToken()

	if p.curToken.Type != TokenDataset {
		return nil, fmt.Errorf("expected DATASET after OF")
	}
	p.nextToken()

	// Parse dataset names (space-separated)
	var datasetNames []string
	for {
		name, err := p.parseQuotedString()
		if err != nil {
			return nil, fmt.Errorf("expected dataset name: %w", err)
		}
		datasetNames = append(datasetNames, name)

		p.nextToken()

		if p.curToken.Type == TokenComma {
			return nil, fmt.Errorf("syntax error: dataset names must be space-separated, not comma-separated (got %q after %q)", "'", name)
		}
		// Stop at semicolon or non-quoted (dataset name must be quoted)
		if p.curToken.Type == TokenSemicolon {
			break
		}
		// If next token is not a quoted string, stop parsing dataset names
		if p.curToken.Type != TokenQuotedString {
			break
		}
	}

	cmd := NewCommand("dev_get_metadata")
	cmd.Params["dataset_names"] = datasetNames

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}

	return cmd, nil
}

func (p *Parser) parseDevExplain() (*Command, error) {
	p.nextToken() // consume EXPLAIN

	switch p.curToken.Type {
	case TokenChunk:
		return p.parseDevChunk(true)
	default:
		return nil, fmt.Errorf("expected CHUNK after EXPLAIN")
	}
}

func (p *Parser) parseDevChunk(explain bool) (*Command, error) {
	p.nextToken() // consume CHUNK

	filename, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected filename: %w", err)
	}
	p.nextToken()

	if p.curToken.Type != TokenWith {
		return nil, fmt.Errorf("expected WITH after filename")
	}
	p.nextToken()

	dsl, err := p.parseQuotedString()
	if err != nil {
		return nil, fmt.Errorf("expected chunk options file: %w", err)
	}

	// Semicolon is optional
	if p.curToken.Type == TokenSemicolon {
		p.nextToken()
	}
	p.nextToken()

	cmd := NewCommand("dev_chunk")
	cmd.Params["dsl"] = dsl
	cmd.Params["filename"] = filename
	cmd.Params["explain"] = explain

	return cmd, nil
}
