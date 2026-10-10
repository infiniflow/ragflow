import { isEqual } from 'lodash';
import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import useGraphStore from '../../store';
import { VariableAggregatorFormSchemaType } from './schema';

export function useWatchFormChange(
  id?: string,
  form?: UseFormReturn<VariableAggregatorFormSchemaType>,
) {
  const values = useWatch({ control: form?.control });
  const { getNode, replaceNodeForm } = useGraphStore((state) => state);

  useEffect(() => {
    if (!id) {
      return;
    }

    const outputs = values.groups?.reduce(
      (pre, cur) => {
        if (cur.group_name) {
          pre[cur.group_name] = {
            type: cur.type,
          };
        }

        return pre;
      },
      {} as Record<string, Record<string, any>>,
    );

    const nextValues = { ...values, outputs: outputs ?? {} };

    // Gate on divergence from the store rather than formState.isDirty:
    // isDirty compares against the mount-time defaultValues snapshot, so
    // returning to it (add a group, then remove it) reads as "clean" and
    // the deletion would never reach the store.
    if (!isEqual(getNode(id)?.data.form, nextValues)) {
      replaceNodeForm(id, nextValues);
    }
  }, [getNode, id, replaceNodeForm, values]);
}
