import { useState } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';

import { FormSchemaType } from '@/pages/user-setting/compilation-templates/edit-template/schema';

export function useTemplatePreviewSheets(
  form: UseFormReturn<FormSchemaType>,
  selectedTemplateIndex: number,
) {
  const [jsonSheetOpen, setJsonSheetOpen] = useState(false);
  const [workflowSheetOpen, setWorkflowSheetOpen] = useState(false);

  const allFormValues = useWatch({ control: form.control });

  const templateName = useWatch({
    control: form.control,
    name: `templates.${selectedTemplateIndex}.name`,
  });

  return {
    jsonSheetOpen,
    setJsonSheetOpen,
    workflowSheetOpen,
    setWorkflowSheetOpen,
    allFormValues,
    templateName,
  };
}
