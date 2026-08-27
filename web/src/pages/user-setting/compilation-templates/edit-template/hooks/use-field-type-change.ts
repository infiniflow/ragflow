import {
  ICompilationTemplateField,
  ICompilationTemplateSection,
} from '@/interfaces/database/compilation-template';
import { useCallback } from 'react';
import { UseFormReturn } from 'react-hook-form';

import { FormSchemaType } from '../schema';

type FieldLike = {
  value: string;
  onChange: (value: string) => void;
};

type UseFieldTypeChangeOptions = {
  form: UseFormReturn<FormSchemaType>;
  builtinSection?: ICompilationTemplateSection;
  fieldsPath: `templates.${number}.config.${string}.fields`;
  index: number;
};

const FIELD_SYNC_KEYS: Array<keyof ICompilationTemplateField> = [
  'description',
  'rule',
];

export const useFieldTypeChange = ({
  form,
  builtinSection,
  fieldsPath,
  index,
}: UseFieldTypeChangeOptions) => {
  return useCallback(
    (field: FieldLike, value: string) => {
      if (!value || value === field.value) {
        field.onChange(value);
        return;
      }

      const matchedField = builtinSection?.fields?.find(
        (builtinField) => builtinField.type === value,
      );

      const currentField = form.getValues(`${fieldsPath}.${index}`);
      const nextField: Record<string, string> = {
        ...currentField,
        type: value,
      };

      if (matchedField) {
        FIELD_SYNC_KEYS.forEach((key) => {
          if (key in matchedField) {
            nextField[key] = matchedField[key] ?? '';
          }
        });
      }

      form.setValue(`${fieldsPath}.${index}`, nextField, {
        shouldValidate: false,
        shouldDirty: true,
        shouldTouch: true,
      });
    },
    [builtinSection, fieldsPath, form, index],
  );
};
