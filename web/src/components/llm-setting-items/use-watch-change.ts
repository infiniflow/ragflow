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

import { settledModelVariableMap } from '@/constants/knowledge';
import { AgentFormContext } from '@/pages/agent/context';
import useGraphStore from '@/pages/agent/store';
import { setChatVariableEnabledFieldValuePage } from '@/utils/chat';
import { useCallback, useContext } from 'react';
import { useFormContext } from 'react-hook-form';

export function useHandleFreedomChange(
  getFieldWithPrefix: (name: string) => string,
) {
  const form = useFormContext();
  const node = useContext(AgentFormContext);
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  const setLLMParameters = useCallback(
    (values: Record<string, any>, withPrefix: boolean) => {
      for (const key in values) {
        if (Object.prototype.hasOwnProperty.call(values, key)) {
          const realKey = getFieldWithPrefix(key);
          const element = values[key as keyof typeof values];

          form.setValue(withPrefix ? realKey : key, element);
        }
      }
    },
    [form, getFieldWithPrefix],
  );

  const handleChange = useCallback(
    (parameter: string) => {
      const currentValues = { ...form.getValues() };
      const values =
        settledModelVariableMap[
          parameter as keyof typeof settledModelVariableMap
        ];

      const nextValues = { ...currentValues, ...values };

      if (node?.id) {
        updateNodeForm(node?.id, nextValues);
      }

      const variableCheckBoxFieldMap = setChatVariableEnabledFieldValuePage();

      setLLMParameters(values, true);
      setLLMParameters(variableCheckBoxFieldMap, false);
    },
    [form, node?.id, setLLMParameters, updateNodeForm],
  );

  return handleChange;
}
