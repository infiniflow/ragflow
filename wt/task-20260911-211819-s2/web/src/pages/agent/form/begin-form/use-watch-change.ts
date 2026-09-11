import { isEmpty, omit } from 'lodash';
import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { AgentDialogueMode } from '../../constant';
import { BeginQuery } from '../../interface';
import useGraphStore from '../../store';
import { BeginFormSchemaType } from './schema';

export function transferInputsArrayToObject(inputs: BeginQuery[] = []) {
  return inputs.reduce<Record<string, Omit<BeginQuery, 'key'>>>((pre, cur) => {
    pre[cur.key] = omit(cur, 'key');

    return pre;
  }, {});
}

function transformRequestSchemaToOutput(schema: BeginFormSchemaType['schema']) {
  const outputs: Record<string, any> = {};

  Object.entries(schema || {}).forEach(([key, value]) => {
    if (Array.isArray(value)) {
      for (const cur of value) {
        outputs[`${key}.${cur.key}`] = { type: cur.type };
      }
    }
  });

  return outputs;
}

export function useWatchFormChange(id?: string, form?: UseFormReturn) {
  let values = useWatch({ control: form?.control });
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  useEffect(() => {
    if (id) {
      values = form?.getValues() || {};

      let outputs: Record<string, any> = {};

      // For webhook mode, use schema properties as direct outputs
      // Each property (body, headers, query) should be able to show secondary menu
      if (
        values.mode === AgentDialogueMode.Webhook &&
        !isEmpty(values.schema)
      ) {
        outputs = transformRequestSchemaToOutput(values.schema);
      }

      const nextValues = {
        ...values,
        inputs: transferInputsArrayToObject(values.inputs),
        outputs,
      };

      updateNodeForm(id, nextValues);
    }
  }, [form?.formState.isDirty, id, updateNodeForm, values]);
}
