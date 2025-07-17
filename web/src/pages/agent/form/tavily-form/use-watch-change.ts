import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import useGraphStore from '../../store';
import { convertToStringArray } from '../../utils';

export function useWatchFormChange(id?: string, form?: UseFormReturn<any>) {
  let values = useWatch({ control: form?.control });
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  useEffect(() => {
    // Manually triggered form updates are synchronized to the canvas
    if (id) {
      values = form?.getValues();
      let nextValues: any = {
        ...values,
        include_domains: convertToStringArray(values.include_domains),
        exclude_domains: convertToStringArray(values.exclude_domains),
      };

      updateNodeForm(id, nextValues);
    }
  }, [form?.formState.isDirty, id, updateNodeForm, values]);
}
