import useGraphStore from '@/pages/agent/store';
import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';

export function useWatchFormChange(id?: string, form?: UseFormReturn<any>) {
  let values = useWatch({ control: form?.control });
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  useEffect(() => {
    // Manually triggered form updates are synchronized to the canvas
    if (id) {
      values = form?.getValues() || {};
      let nextValues: any = values;

      updateNodeForm(id, nextValues);
    }
  }, [id, updateNodeForm, values]);
}

export function useWatchNameFormChange(id?: string, form?: UseFormReturn<any>) {
  let values = useWatch({ control: form?.control });
  const updateNodeName = useGraphStore((state) => state.updateNodeName);

  useEffect(() => {
    // Manually triggered form updates are synchronized to the canvas
    if (id) {
      updateNodeName(id, values.name);
    }
  }, [id, updateNodeName, values]);
}
