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

import { CompilationTemplateKind } from '@/constants/compilation';
import {
  buildConfigFromBuiltin,
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

describe('entity-mode page selection', () => {
  const template = {
    id: 'brands',
    name: 'Brand wiki',
    description: '',
    kind: CompilationTemplateKind.Artifacts,
    config: {
      mode: 'entity' as const,
      entity: { fields: [{ type: 'brand' }] },
    },
  };

  it('round-trips the setting through both builtin and saved templates', () => {
    const config = { ...template.config, configured_entity_pages_only: true };
    const builtin = buildConfigFromBuiltin(
      { ...template, display_name: template.name, config },
      template.kind,
    );
    const saved = transformDetailToForm({ ...template, config });
    for (const formConfig of [builtin, saved.config]) {
      const payload = transformTemplateToPayload({
        ...saved,
        config: formConfig,
      });
      expect(payload.config.configured_entity_pages_only).toBe(true);
      expect(payload.config.entity?.fields).toEqual([{ type: 'brand' }]);
    }
  });

  it('omits the restriction for existing templates, topic mode and an unchecked switch', () => {
    const form = transformDetailToForm(template);
    for (const config of [
      form.config,
      { ...form.config, configured_entity_pages_only: false },
      { ...form.config, mode: 'topic', configured_entity_pages_only: true },
    ]) {
      expect(
        transformTemplateToPayload({ ...form, config }).config,
      ).not.toHaveProperty('configured_entity_pages_only');
    }
  });
});
