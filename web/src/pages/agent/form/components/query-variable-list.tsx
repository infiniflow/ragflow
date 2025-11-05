import { BlockButton, Button } from '@/components/ui/button';
import { X } from 'lucide-react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { JsonSchemaDataType } from '../../constant';
import { QueryVariable } from './query-variable';

type QueryVariableListProps = {
  types?: JsonSchemaDataType[];
};
export function QueryVariableList({ types }: QueryVariableListProps) {
  const { t } = useTranslation();
  const form = useFormContext();
  const name = 'inputs';

  const { fields, remove, append } = useFieldArray({
    name: name,
    control: form.control,
  });

  return (
    <div className="space-y-5">
      {fields.map((field, index) => {
        const nameField = `${name}.${index}.input`;

        return (
          <div key={field.id} className="flex items-center gap-2">
            <QueryVariable
              name={nameField}
              hideLabel
              className="flex-1"
              types={types}
            ></QueryVariable>
            <Button variant={'ghost'} onClick={() => remove(index)}>
              <X className="text-text-sub-title-invert " />
            </Button>
          </div>
        );
      })}

      <BlockButton onClick={() => append({ input: '' })}>
        {t('common.add')}
      </BlockButton>
    </div>
  );
}
