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

import { normalizeParserConfig } from './parser-config-utils';

const CHUNKER = 'GeneralChunker:SixApplesFall';
const EXTRACTOR = 'Extractor:AutoExtractDefault';

describe('normalizeParserConfig', () => {
  it('drops flat parser-level keys and keeps only component-scoped keys', () => {
    const out = normalizeParserConfig({
      chunk_token_num: 256,
      delimiter: '\n',
      auto_keywords: 2,
      layout_recognize: 'DeepDOC',
      [CHUNKER]: { chunk_token_size: 512 },
    }) as Record<string, unknown>;

    expect(out['chunk_token_num']).toBeUndefined();
    expect(out['delimiter']).toBeUndefined();
    expect(out['auto_keywords']).toBeUndefined();
    expect(out[CHUNKER]).toBeDefined();
  });

  it('re-homes chunk_token_num onto the chunker node as chunk_token_size', () => {
    const out = normalizeParserConfig({
      chunk_token_num: 256,
    }) as Record<string, unknown>;

    expect((out[CHUNKER] as Record<string, unknown>).chunk_token_size).toBe(
      256,
    );
  });

  it('re-homes metadata onto the Extractor node', () => {
    const metadata = { enabled: true, metadata: [] };
    const out = normalizeParserConfig({
      metadata,
    }) as Record<string, unknown>;

    expect((out[EXTRACTOR] as Record<string, unknown>).metadata).toEqual(
      metadata,
    );
  });

  it('writes parent_child with use_parent_child:true when enable_children is true', () => {
    const out = normalizeParserConfig({
      enable_children: true,
      children_delimiter: '###',
    }) as Record<string, unknown>;

    const parentChild = (out[CHUNKER] as Record<string, unknown>)
      .parent_child as Record<string, unknown>;
    expect(parentChild.use_parent_child).toBe(true);
    expect(parentChild.children_delimiter).toBe('###');
  });

  it('explicitly disables parent_child when enable_children is false even if a node already enabled it (F2)', () => {
    const out = normalizeParserConfig({
      enable_children: false,
      [CHUNKER]: {
        chunk_token_size: 512,
        parent_child: { use_parent_child: true, children_delimiter: '###' },
      },
    }) as Record<string, unknown>;

    const chunker = out[CHUNKER] as Record<string, unknown>;
    expect(chunker.chunk_token_size).toBe(512);
    const parentChild = chunker.parent_child as Record<string, unknown>;
    expect(parentChild.use_parent_child).toBe(false);
  });

  it('leaves an existing parent_child untouched when enable_children is absent', () => {
    const existing = { use_parent_child: true, children_delimiter: '###' };
    const out = normalizeParserConfig({
      [CHUNKER]: { chunk_token_size: 512, parent_child: existing },
    }) as Record<string, unknown>;

    const chunker = out[CHUNKER] as Record<string, unknown>;
    expect(chunker.parent_child).toEqual(existing);
  });
});
