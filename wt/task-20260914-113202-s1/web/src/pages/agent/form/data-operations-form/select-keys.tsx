import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { X } from 'lucide-react';
import { ReactNode } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { DynamicFormHeader } from '../components/dynamic-fom-header';
import { PromptEditor } from '../components/prompt-editor';

type SelectKeysProps = {
  name: string;
  label: ReactNode;
  tooltip?: string;
};
export function SelectKeys({ name, label, tooltip }: SelectKeysProps) {
  const form = useFormContext();

  const { fields, remove, append } = useFieldArray({
    name: name,
    control: form.control,
  });

  return (
    <section className="space-y-2">
      <DynamicFormHeader
        label={label}
        tooltip={tooltip}
        onClick={() => append({ name: '' })}
      ></DynamicFormHeader>
      <div className="space-y-5">
        {fields.map((field, index) => {
          const nameField = `${name}.${index}.name`;

          return (
            <div key={field.id} className="flex items-center gap-2">
              <RAGFlowFormItem name={nameField} className="flex-1">
                <PromptEditor showToolbar={false} multiLine={false} />
              </RAGFlowFormItem>
              <Button variant={'ghost'} onClick={() => remove(index)}>
                <X />
              </Button>
            </div>
          );
        })}
      </div>
    </section>
  );
}
