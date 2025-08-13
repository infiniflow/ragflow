import { settledModelVariableMap } from '@/constants/knowledge';
import { AgentFormContext } from '@/pages/agent/context';
import useGraphStore from '@/pages/agent/store';
import { useCallback, useContext } from 'react';
import { useFormContext } from 'react-hook-form';

export function useHandleFreedomChange(
  getFieldWithPrefix: (name: string) => string,
) {
  const form = useFormContext();
  const node = useContext(AgentFormContext);
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

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

      for (const key in values) {
        if (Object.prototype.hasOwnProperty.call(values, key)) {
          const realKey = getFieldWithPrefix(key);
          const element = values[key as keyof typeof values];

          form.setValue(realKey, element);
        }
      }
    },
    [form, getFieldWithPrefix, node?.id, updateNodeForm],
  );

  return handleChange;
}
