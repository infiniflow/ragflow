import { RAGFlowFormItem } from '@/components/ragflow-form';
import { BlockButton, Button } from '@/components/ui/button';
import { FormLabel } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { X } from 'lucide-react';
import { ReactNode } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

type SelectKeysProps = {
  name: string;
  label: ReactNode;
  tooltip?: string;
};
export function SelectKeys({ name, label, tooltip }: SelectKeysProps) {
  const { t } = useTranslation();
  const form = useFormContext();

  const { fields, remove, append } = useFieldArray({
    name: name,
    control: form.control,
  });

  return (
    <section className="space-y-2">
      <FormLabel tooltip={tooltip}>{label}</FormLabel>
      <div className="space-y-5">
        {fields.map((field, index) => {
          const nameField = `${name}.${index}.name`;

          return (
            <div key={field.id} className="flex items-center gap-2">
              <RAGFlowFormItem name={nameField} className="flex-1">
                <Input></Input>
              </RAGFlowFormItem>
              <Button variant={'ghost'} onClick={() => remove(index)}>
                <X />
              </Button>
            </div>
          );
        })}
      </div>

      <BlockButton onClick={() => append({ name: '' })}>
        {t('common.add')}
      </BlockButton>
    </section>
  );
}
