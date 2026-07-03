import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { Plus } from 'lucide-react';
import { ArrayPath, useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { FormSchemaType } from '@/pages/user-setting/compilation-templates/edit-template/schema';

import { FieldCard } from './field-card';

type SectionFieldGridProps = {
  fieldsPath: ArrayPath<FormSchemaType>;
  onOpenAddField: () => void;
  onEditField: (index: number) => void;
};

export function SectionFieldGrid({
  fieldsPath,
  onOpenAddField,
  onEditField,
}: SectionFieldGridProps) {
  const { t } = useTranslation();
  const form = useFormContext<FormSchemaType>();
  const { fields, remove } = useFieldArray({
    control: form.control,
    name: fieldsPath,
  });

  return (
    <section className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
      {fields.map((field, index) => {
        const fieldValue =
          (form.getValues(`${fieldsPath}.${index}`) as
            | Record<string, string>
            | undefined) ?? {};
        return (
          <FieldCard
            key={field.id}
            index={index}
            field={fieldValue}
            onEdit={() => onEditField(index)}
            onDelete={() => remove(index)}
          />
        );
      })}

      <Card
        role="button"
        tabIndex={0}
        onClick={onOpenAddField}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            onOpenAddField();
          }
        }}
        className={cn(
          'border-border-button bg-transparent border-dashed flex flex-col items-center justify-center gap-2 min-h-[140px] cursor-pointer',
          'hover:border-border-accent hover:text-text-primary text-text-secondary',
        )}
      >
        <CardContent className="flex flex-col items-center justify-center gap-2 p-4">
          <Plus className="size-6" />
          <span className="text-sm font-medium">{t('setting.addField')}</span>
        </CardContent>
      </Card>
    </section>
  );
}
