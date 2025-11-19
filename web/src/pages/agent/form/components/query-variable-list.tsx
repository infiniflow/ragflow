import { Button } from '@/components/ui/button';
import { X } from 'lucide-react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { JsonSchemaDataType } from '../../constant';
import {
  flatOptions,
  useFilterQueryVariableOptionsByTypes,
} from '../../hooks/use-get-begin-query';
import { DynamicFormHeader, FormListHeaderProps } from './dynamic-fom-header';
import { QueryVariable } from './query-variable';

type QueryVariableListProps = {
  types?: JsonSchemaDataType[];
} & FormListHeaderProps;
export function QueryVariableList({
  types,
  label,
  tooltip,
}: QueryVariableListProps) {
  const form = useFormContext();
  const name = 'query';

  let options = useFilterQueryVariableOptionsByTypes(types);

  const secondOptions = flatOptions(options);

  const { fields, remove, append } = useFieldArray({
    name: name,
    control: form.control,
  });

  return (
    <section className="space-y-2">
      <DynamicFormHeader
        label={label}
        tooltip={tooltip}
        onClick={() => append({ input: secondOptions.at(0)?.value })}
        disabled={!secondOptions.length}
      ></DynamicFormHeader>
      <div className="space-y-5">
        {fields.map((field, index) => {
          const nameField = `${name}.${index}.input`;

          return (
            <div key={field.id} className="flex gap-2">
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
      </div>
    </section>
  );
}
