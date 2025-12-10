import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Trash2 } from 'lucide-react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { DynamicFormHeader, FormListHeaderProps } from './dynamic-fom-header';

type DynamicStringFormProps = { name: string } & FormListHeaderProps;
export function DynamicStringForm({ name, label }: DynamicStringFormProps) {
  const form = useFormContext();

  const { fields, append, remove } = useFieldArray({
    name: name,
    control: form.control,
  });

  return (
    <section>
      <DynamicFormHeader
        label={label}
        onClick={() => append({ value: '' })}
      ></DynamicFormHeader>
      <div className="space-y-4">
        {fields.map((field, index) => (
          <div key={field.id} className="flex items-center gap-2">
            <RAGFlowFormItem
              name={`${name}.${index}.value`}
              label="delimiter"
              labelClassName="!hidden"
              className="flex-1 !m-0"
            >
              <Input className="!m-0"></Input>
            </RAGFlowFormItem>
            <Button
              type="button"
              variant={'ghost'}
              onClick={() => remove(index)}
            >
              <Trash2 />
            </Button>
          </div>
        ))}
      </div>
    </section>
  );
}
