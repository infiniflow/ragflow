import { ICompilationTemplateBuiltin } from '@/interfaces/database/compilation-template';
import { UseFormReturn } from 'react-hook-form';

import { FormSchemaType } from '../schema';
import { buildConfigFromBuiltin } from '../utils';

type FieldLike = {
  value: string;
  onChange: (value: string) => void;
};

type UseTemplateKindChangeOptions = {
  form: UseFormReturn<FormSchemaType>;
  index: number;
  builtins: ICompilationTemplateBuiltin[];
};

export const useTemplateKindChange = ({
  form,
  index,
  builtins,
}: UseTemplateKindChangeOptions) => {
  return (field: FieldLike, value: string) => {
    if (value && value !== field.value) {
      const builtinTemplate = builtins.find(
        (template) => template.kind === value,
      );
      if (builtinTemplate) {
        form.setValue(
          `templates.${index}.config`,
          buildConfigFromBuiltin(
            builtinTemplate,
            value,
            form.getValues(`templates.${index}.llm_id`),
          ),
          { shouldValidate: false },
        );
      }
    }
    field.onChange(value);
  };
};
