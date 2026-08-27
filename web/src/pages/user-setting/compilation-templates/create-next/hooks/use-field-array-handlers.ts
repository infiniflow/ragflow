import { useCallback } from 'react';
import { ArrayPath, UseFormReturn } from 'react-hook-form';

import { FormSchemaType } from '@/pages/user-setting/compilation-templates/edit-template/schema';

export const useFieldArrayHandlers = (
  form: UseFormReturn<FormSchemaType>,
  activeFieldsPath: ArrayPath<FormSchemaType>,
  editingFieldIndex: number | null,
  setEditingFieldIndex: (index: number | null) => void,
) => {
  const handleAddField = useCallback(
    (field: Record<string, string>) => {
      const currentFields =
        (form.getValues(activeFieldsPath) as
          | Record<string, string>[]
          | undefined) ?? [];
      if (editingFieldIndex !== null) {
        const newFields = [...currentFields];
        newFields[editingFieldIndex] = field;
        form.setValue(activeFieldsPath, newFields, {
          shouldValidate: false,
          shouldDirty: true,
          shouldTouch: true,
        });
      } else {
        form.setValue(activeFieldsPath, [...currentFields, field], {
          shouldValidate: false,
          shouldDirty: true,
          shouldTouch: true,
        });
      }
      setEditingFieldIndex(null);
    },
    [activeFieldsPath, editingFieldIndex, form, setEditingFieldIndex],
  );

  return { handleAddField };
};
