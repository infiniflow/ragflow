import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { Plus } from 'lucide-react';
import { useFieldArray, useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { FieldCard } from './field-card';

type SectionFieldGridProps = {
  fieldsPath: string;
  sectionName: string;
  onOpenAddField: () => void;
  onEditField: (index: number) => void;
};

export function SectionFieldGrid({
  fieldsPath,
  sectionName,
  onOpenAddField,
  onEditField,
}: SectionFieldGridProps) {
  const { t } = useTranslation();
  const form = useFormContext();
  const { fields, remove } = useFieldArray({
    control: form.control,
    name: fieldsPath,
  });

  const isTypedSection = sectionName === 'entity' || sectionName === 'relation';

  const currentFields = useWatch({
    control: form.control,
    name: fieldsPath,
  }) as Record<string, string>[] | undefined;

  return (
    <section className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
      {fields.map((field, index) => {
        const fieldValue = currentFields?.[index] ?? {};
        return (
          <FieldCard
            key={field.id}
            title={isTypedSection ? fieldValue.type : undefined}
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
