import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import useGraphStore from '../../store';
import { OutputArray, OutputObject } from './interface';

export function transferToObject(list: OutputArray) {
  return list.reduce<OutputObject>((pre, cur) => {
    pre[cur.name] = { ref: cur.ref, type: cur.type };
    return pre;
  }, {});
}

export function useWatchFormChange(id?: string, form?: UseFormReturn) {
  let values = useWatch({ control: form?.control });
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  useEffect(() => {
    // Manually triggered form updates are synchronized to the canvas
    if (id && form?.formState.isDirty) {
      values = form?.getValues();
      console.log('🚀 ~ useEffect ~ values:', values);
      let nextValues: any = {
        ...values,
        outputs: transferToObject(values.outputs),
      };

      updateNodeForm(id, nextValues);
    }
  }, [form?.formState.isDirty, id, updateNodeForm, values]);
}
