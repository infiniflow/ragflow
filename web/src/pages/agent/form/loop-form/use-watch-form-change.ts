import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { IOutputs } from '../../interface';
import useGraphStore from '../../store';
import { LoopFormSchemaType } from './schema';

export function useWatchFormChange(
  id?: string,
  form?: UseFormReturn<LoopFormSchemaType>,
) {
  let values = useWatch({ control: form?.control });
  const { replaceNodeForm } = useGraphStore((state) => state);

  useEffect(() => {
    if (id) {
      let nextValues = {
        ...values,
        outputs: values.loop_variables?.reduce((pre, cur) => {
          const variable = cur.variable;
          if (variable) {
            pre[variable] = {
              type: 'string',
              value: '',
            };
          }
          return pre;
        }, {} as IOutputs),
      };

      replaceNodeForm(id, nextValues);
    }
  }, [form?.formState.isDirty, id, replaceNodeForm, values]);
}
