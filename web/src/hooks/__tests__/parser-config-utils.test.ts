/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { normalizeParserConfig } from '@/hooks/parser-config-utils';

const baseConfig = {
  chunk_token_num: 128,
  layout_recognize: 'DeepDOC',
  delimiter: '\n',
  metadata: { enabled: true, metadata: [], built_in_metadata: [] },
  enable_children: true,
  children_delimiter: '!?;',
  use_parent_child: true,
  graphrag: { use_graphrag: false },
  raptor: { use_raptor: false },
};

describe('normalizeParserConfig backend awareness', () => {
  beforeEach(() => {
    mockIsGoBackend = true;
  });

  it('scopes metadata and parent_child onto component nodes and drops every flat key for the Go backend', () => {
    const out = normalizeParserConfig({
      ...baseConfig,
    }) as Record<string, any>;

    expect(out['Extractor:AutoExtractDefault']).toEqual({
      metadata: { enabled: true, metadata: [], built_in_metadata: [] },
    });
    expect(out['GeneralChunker:SixApplesFall']).toEqual({
      chunk_token_size: 128,
      parent_child: { children_delimiter: '!?;', use_parent_child: true },
    });
    // No flat transport keys survive for the Go backend.
    expect(out).not.toHaveProperty('metadata');
    expect(out).not.toHaveProperty('parent_child');
    // The Go backend reads chunk_token_size off the chunker node, never a flat
    // chunk_token_num, so the flat key must be gone and the value re-homed.
    expect(out).not.toHaveProperty('chunk_token_num');
    expect(out).not.toHaveProperty('delimiter');
    expect(out).not.toHaveProperty('layout_recognize');
    expect(out).not.toHaveProperty('auto_keywords');
    // graphrag/raptor are always stripped.
    expect(out).not.toHaveProperty('graphrag');
    expect(out).not.toHaveProperty('raptor');
  });

  it('does not scope anything when metadata/parent_child are absent but still re-homes chunk_token_num', () => {
    const out = normalizeParserConfig({
      chunk_token_num: 128,
      layout_recognize: 'DeepDOC',
      delimiter: '\n',
    }) as Record<string, any>;

    expect(out).not.toHaveProperty('Extractor:AutoExtractDefault');
    expect(out).not.toHaveProperty('parent_child');
    expect(out).not.toHaveProperty('metadata');
    // No flat parser-level key survives for the Go backend.
    expect(out).not.toHaveProperty('chunk_token_num');
    expect(out).not.toHaveProperty('delimiter');
    // The chunk size is still re-homed onto the chunker node.
    expect(out['GeneralChunker:SixApplesFall']).toEqual({
      chunk_token_size: 128,
    });
  });
});
