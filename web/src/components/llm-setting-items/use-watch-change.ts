import { settledModelVariableMap } from '@/constants/knowledge';
import { FlowFormContext } from '@/pages/agent/context';
import useGraphStore from '@/pages/agent/store';
import { useCallback, useContext } from 'react';
import { useFormContext } from 'react-hook-form';

export function useHandleFreedomChange() {
  const form = useFormContext();
  const node = useContext(FlowFormContext);
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
          const element = values[key];

          form.setValue(key, element);
        }
      }
    },
    [form, node, updateNodeForm],
  );

  return handleChange;
}
