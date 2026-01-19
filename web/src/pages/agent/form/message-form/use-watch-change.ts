import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import useGraphStore from '../../store';
import { convertToStringArray } from '../../utils';

export function useWatchFormChange(id?: string, form?: UseFormReturn) {
  let values = useWatch({ control: form?.control });
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  useEffect(() => {
    // Manually triggered form updates are synchronized to the canvas
    if (id && form?.formState.isDirty) {
      values = form?.getValues();
      let nextValues: any = values;

      nextValues = {
        ...values,
        content: convertToStringArray(values.content),
      };

      updateNodeForm(id, nextValues);
    }
  }, [form?.formState.isDirty, id, updateNodeForm, values]);
}
