import { ISwitchCondition } from '@/interfaces/database/agent';
import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import useGraphStore from '../../store';

export function useWatchFormChange(id?: string, form?: UseFormReturn) {
  let values = useWatch({ control: form?.control });
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  useEffect(() => {
    // Manually triggered form updates are synchronized to the canvas
    console.log('🚀 ~ useWatchFormChange ~ values:', form?.formState.isDirty);
    if (id) {
      values = form?.getValues() || {};
      let nextValues: any = {
        ...values,
        conditions:
          values?.conditions?.map((x: ISwitchCondition) => ({ ...x })) ?? [], // Changing the form value with useFieldArray does not change the array reference
      };

      updateNodeForm(id, nextValues);
    }
  }, [form?.formState.isDirty, id, updateNodeForm, values]);
}
