import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Plus, Trash2 } from 'lucide-react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useGetVariableLabelOrTypeByValue } from '../../hooks/use-get-begin-query';
import { QueryVariable } from '../components/query-variable';
import { NameInput } from './name-input';

type DynamicGroupVariableProps = {
  name: string;
  parentIndex: number;
  removeParent: (index: number) => void;
};

export function DynamicGroupVariable({
  name,
  parentIndex,
  removeParent,
}: DynamicGroupVariableProps) {
  const form = useFormContext();

  const variableFieldName = `${name}.variables`;

  const { getType } = useGetVariableLabelOrTypeByValue();

  const { fields, remove, append } = useFieldArray({
    name: variableFieldName,
    control: form.control,
  });

  const firstValue = form.getValues(`${variableFieldName}.0.value`);
  const firstType = getType(firstValue);

  return (
    <section className="py-3 group space-y-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <RAGFlowFormItem name={`${name}.group_name`} className="w-32">
            {(field) => (
              <NameInput
                value={field.value}
                onChange={field.onChange}
              ></NameInput>
            )}
          </RAGFlowFormItem>
          {/* Use a hidden form to store data types; otherwise, data loss may occur. */}
          <RAGFlowFormItem name={`${name}.type`} className="hidden">
            <Input></Input>
          </RAGFlowFormItem>
          <Button
            variant={'ghost'}
            type="button"
            className="hidden group-hover:block"
            onClick={() => removeParent(parentIndex)}
          >
            <Trash2 />
          </Button>
        </div>
        <div className="flex gap-2 items-center">
          {firstType && (
            <span className="text-text-secondary border px-1 rounded-md">
              {firstType}
            </span>
          )}
          <Button
            variant={'ghost'}
            type="button"
            onClick={() => append({ value: '' })}
          >
            <Plus />
          </Button>
        </div>
      </div>

      <section className="space-y-3">
        {fields.map((field, index) => (
          <div key={field.id} className="flex gap-2 items-center">
            <QueryVariable
              name={`${variableFieldName}.${index}.value`}
              className="flex-1 min-w-0"
              hideLabel
              types={firstType && fields.length > 1 ? [firstType] : []}
              onChange={(val) => {
                const type = getType(val);
                if (type && index === 0) {
                  form.setValue(`${name}.type`, type, {
                    shouldDirty: true,
                  });
                }
              }}
            ></QueryVariable>
            <Button
              variant={'ghost'}
              type="button"
              onClick={() => remove(index)}
            >
              <Trash2 />
            </Button>
          </div>
        ))}
      </section>
    </section>
  );
}
