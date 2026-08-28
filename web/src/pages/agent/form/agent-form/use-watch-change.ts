import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { PromptRole } from '../../constant';
import useGraphStore from '../../store';

export function useWatchFormChange(id?: string, form?: UseFormReturn<any>) {
  let values = useWatch({ control: form?.control });
  const { updateNodeForm, markNodeFormEdited } = useGraphStore((state) => state);

  useEffect(() => {
    // Manually triggered form updates are synchronized to the canvas
    if (id && form?.formState.isDirty) {
      // Edited nodes take part in the save-time validation gate.
      markNodeFormEdited(id);
      values = form?.getValues();
      const nextValues: any = {
        ...values,
        prompts: [{ role: PromptRole.User, content: values.prompts }],
      };

      updateNodeForm(id, nextValues);
    }
  }, [form?.formState.isDirty, id, markNodeFormEdited, updateNodeForm, values]);
}
