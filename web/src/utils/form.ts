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

import { variableEnabledFieldMap } from '@/constants/chat';
import {
  ModelVariableType,
  settledModelVariableMap,
} from '@/constants/knowledge';
import { TFunction } from 'i18next';
import { camelCase, isNil } from 'lodash';
import omit from 'lodash/omit';

// chat model setting and generate operator
export const excludeUnEnabledVariables = (
  values: any = {},
  prefix = 'llm_setting.',
) => {
  const unEnabledFields: Array<keyof typeof variableEnabledFieldMap> =
    Object.keys(variableEnabledFieldMap).filter((key) => !values[key]) as Array<
      keyof typeof variableEnabledFieldMap
    >;

  return unEnabledFields.map(
    (key) => `${prefix}${variableEnabledFieldMap[key]}`,
  );
};

// chat model setting and generate operator
export const removeUselessFieldsFromValues = (values: any, prefix = '') => {
  const nextValues: any = omit(values, [
    ...Object.keys(variableEnabledFieldMap),
    `${prefix}parameter`,
    ...excludeUnEnabledVariables(values, prefix),
  ]);

  return nextValues;
};

// `parameter` (the "freedom" preset select) is a derived field: picking a
// preset only copies the preset's sampling values into the form, and the preset
// name itself is never persisted — removeUselessFieldsFromValues strips it on
// every save path. Rebuild it on load by matching the stored values against
// the presets. Only keys present in llmSetting participate: presets force
// maxTokensEnabled off, so a stored preset config legitimately has no
// max_tokens. Values matching no preset — or several, e.g. only max_tokens —
// are reported as 'Custom'.
export function deriveModelPreset(
  llmSetting?: Record<string, any>,
): ModelVariableType | 'Custom' {
  const matches = Object.entries(settledModelVariableMap).filter(
    ([, presetValues]) => {
      const comparableKeys = (
        Object.keys(presetValues) as Array<keyof typeof presetValues>
      ).filter((key) => !isNil(llmSetting?.[key]));

      return (
        comparableKeys.length > 0 &&
        comparableKeys.every((key) => presetValues[key] === llmSetting?.[key])
      );
    },
  );

  return matches.length === 1 ? (matches[0][0] as ModelVariableType) : 'Custom';
}

export function buildOptions(
  data: Record<string, any>,
  t?: TFunction<['translation', ...string[]], undefined>,
  prefix?: string,
  camel: boolean = false,
) {
  if (t) {
    return Object.values(data).map((val) => ({
      label: t(
        `${prefix ? prefix + '.' : ''}${typeof val === 'string' ? (camel ? camelCase(val) : val.toLowerCase()) : val}`,
      ),
      value: val,
    }));
  }
  return Object.values(data).map((val) => ({ label: val, value: val }));
}

export function setLLMSettingEnabledValues(
  initialLlmSetting?: Record<string, any>,
) {
  const values = Object.keys(variableEnabledFieldMap).reduce<
    Record<string, boolean>
  >((pre, field) => {
    pre[field] =
      initialLlmSetting === undefined
        ? false
        : !isNil(
            initialLlmSetting[
              variableEnabledFieldMap[
                field as keyof typeof variableEnabledFieldMap
              ]
            ],
          );
    return pre;
  }, {});
  return values;
}

/**
 * Add prefix to form field name
 * @param prefix - The prefix to add (e.g., 'chat.', 'settings.')
 * @param name - The field name
 * @returns The prefixed field name
 * @example
 * prefixName('chat.', 'icon') // returns 'chat.icon'
 * prefixName('', 'name') // returns 'name'
 */
export function prefixName(prefix: string, name: string): string {
  return `${prefix}${name}`;
}
