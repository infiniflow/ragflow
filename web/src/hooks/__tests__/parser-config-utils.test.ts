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

// Drive the backend variant the same way the existing frontend tests do.
let mockIsGoBackend = true;
jest.mock('@/utils/backend-runtime', () => ({
  getBackendLanguage: () => (mockIsGoBackend ? 'go' : 'python'),
  subscribeBackendLanguage: jest.fn(),
}));

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

  it('scopes metadata and parent_child onto component nodes for the Go backend', () => {
    const out = normalizeParserConfig({
      ...baseConfig,
    }) as Record<string, any>;

    expect(out['Extractor:AutoExtractDefault']).toEqual({
      metadata: { enabled: true, metadata: [], built_in_metadata: [] },
    });
    expect(out['GeneralChunker:SixApplesFall']).toEqual({
      parent_child: { children_delimiter: '!?;', use_parent_child: true },
    });
    // No flat transport keys survive for the Go backend.
    expect(out).not.toHaveProperty('metadata');
    expect(out).not.toHaveProperty('parent_child');
    // Parser-level flat keys are preserved as-is for the built-in path.
    expect(out.chunk_token_num).toBe(128);
    expect(out.delimiter).toBe('\n');
    // graphrag/raptor are always stripped.
    expect(out).not.toHaveProperty('graphrag');
    expect(out).not.toHaveProperty('raptor');
  });

  it('keeps the flat-key contract for the Python backend', () => {
    mockIsGoBackend = false;
    const out = normalizeParserConfig({
      ...baseConfig,
    }) as Record<string, any>;

    expect(out.metadata).toEqual({
      enabled: true,
      metadata: [],
      built_in_metadata: [],
    });
    expect(out.parent_child).toEqual({
      children_delimiter: '!?;',
      use_parent_child: true,
    });
    expect(out).not.toHaveProperty('Extractor:AutoExtractDefault');
    expect(out).not.toHaveProperty('GeneralChunker:SixApplesFall');
  });

  it('does not scope anything when metadata/parent_child are absent', () => {
    const { metadata, enable_children, use_parent_child, ...rest } = baseConfig;
    const out = normalizeParserConfig({ ...rest }) as Record<string, any>;

    expect(out).not.toHaveProperty('Extractor:AutoExtractDefault');
    expect(out).not.toHaveProperty('GeneralChunker:SixApplesFall');
    expect(out).not.toHaveProperty('parent_child');
    expect(out).not.toHaveProperty('metadata');
    expect(out.chunk_token_num).toBe(128);
  });
});
