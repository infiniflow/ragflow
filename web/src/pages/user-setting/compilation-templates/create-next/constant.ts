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

import { FormSchemaType, TemplateSchemaType } from './schema';

export const DefaultFieldKeys = ['type', 'description', 'rule'];

export const FieldKeyOrders = [
  DefaultFieldKeys,
  ['statement', 'subject'],
  ['definition_excerpt', 'term'],
];

export const DefaultTemplateValues: TemplateSchemaType = {
  id: undefined,
  name: '',
  description: '',
  llm_id: '',
  kind: '',
  config: {
    kind: '',
    llm_id: '',
    global_rules: '',
    example: '',
    instruction: '',
    use_blueprint: false,
    plan: true,
    rechunk: false,
    rechunk_rules: '',
  },
};

export const DefaultValues: FormSchemaType = {
  name: '',
  description: '',
  avatar: '',
  templates: [DefaultTemplateValues],
};

export const SectionTitleKeyMap: Record<string, string> = {
  entity: 'setting.entitySpecification',
  relation: 'setting.relationSpecification',
  concept: 'setting.conceptSpecification',
  claim: 'setting.claimSpecification',
};

export const SectionCardFieldMap: Record<
  string,
  { title: string; description: string }
> = {
  entity: { title: 'type', description: 'description' },
  relation: { title: 'type', description: 'description' },
  claim: { title: 'statement', description: 'subject' },
  concept: { title: 'definition_excerpt', description: 'term' },
};

export const SectionPriority = ['entity', 'relation'];

export const FieldLabelKeyMap: Record<string, string> = {
  type: 'setting.fieldType',
  description: 'setting.fieldDescription',
  rule: 'setting.fieldRule',
};

export const FieldRequiredMessageKeyMap: Record<string, string> = {
  type: 'setting.fieldTypeRequired',
  description: 'setting.fieldDescriptionRequired',
};
