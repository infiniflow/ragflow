import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { PromptRole } from '../../constant';
import useGraphStore from '../../store';

export function useWatchFormChange(id?: string, form?: UseFormReturn) {
  let values = useWatch({ control: form?.control });
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  useEffect(() => {
    // Manually triggered form updates are synchronized to the canvas
    if (id && form?.formState.isDirty) {
      values = form?.getValues();
      let nextValues: any = {
        ...values,
        prompts: [{ role: PromptRole.User, content: values.prompts }],
      };

      updateNodeForm(id, nextValues);
    }
  }, [form?.formState.isDirty, id, updateNodeForm, values]);
}
