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

import { buildTemplateSchema } from './schema';
import {
  sortSectionNames,
  transformDetailToForm,
  transformTemplateToPayload,
} from './utils';

describe('sortSectionNames', () => {
  it('prioritizes known sections and preserves remaining order and duplicates', () => {
    expect(
      sortSectionNames([
        'claim',
        'relation',
        'claim',
        'entity',
        'concept',
        'claim',
      ]),
    ).toEqual(['entity', 'relation', 'claim', 'claim', 'concept', 'claim']);
  });
});

describe('Wiki concept page setting', () => {
  it.each([undefined, true, false])(
    'round-trips the saved value %s',
    (saved) => {
      const form = transformDetailToForm({
        id: 'wiki',
        name: 'Product wiki',
        description: '',
        kind: 'wiki',
        config: { mode: 'entity', disable_concept_pages: saved },
      });
      const schema = buildTemplateSchema((key) => key);
      const payload = transformTemplateToPayload(schema.parse(form));
      expect(form.config.disable_concept_pages).toBe(saved === true);
      expect(payload.config.disable_concept_pages).toBe(saved === true);
    },
  );
});
