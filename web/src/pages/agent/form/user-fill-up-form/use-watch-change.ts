import { omit } from 'lodash';
import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { BeginQuery } from '../../interface';
import useGraphStore from '../../store';

function transferInputsArrayToObject(inputs: BeginQuery[] = []) {
  return inputs.reduce<Record<string, Omit<BeginQuery, 'key'>>>((pre, cur) => {
    pre[cur.key] = omit(cur, 'key');

    return pre;
  }, {});
}

export function useWatchFormChange(id?: string, form?: UseFormReturn) {
  let values = useWatch({ control: form?.control });
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  useEffect(() => {
    // TODO: This should only be executed when the form changes
    if (id) {
      values = form?.getValues() || {};

      const inputs = transferInputsArrayToObject(values.inputs);

      const nextValues = {
        ...values,
        inputs,
        outputs: inputs,
      };

      updateNodeForm(id, nextValues);
    }
  }, [form?.formState.isDirty, id, updateNodeForm, values]);
}
