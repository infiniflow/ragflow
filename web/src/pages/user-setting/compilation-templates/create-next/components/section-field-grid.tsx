import { Card, CardContent } from '@/components/ui/card';
import { ICompilationTemplateSection } from '@/interfaces/database/compilation-template';
import { cn } from '@/lib/utils';
import { Plus } from 'lucide-react';
import { useMemo } from 'react';
import {
  ArrayPath,
  useFieldArray,
  useFormContext,
  useWatch,
} from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { FormSchemaType } from '@/pages/user-setting/compilation-templates/create-next/schema';
import { getTypeOptionsFromBuiltinSection } from '@/pages/user-setting/compilation-templates/create-next/utils';

import { FieldCard } from './field-card';

type SectionFieldGridProps = {
  fieldsPath: ArrayPath<FormSchemaType>;
  sectionName: string;
  builtinSection?: ICompilationTemplateSection;
  onOpenAddField: () => void;
  onEditField: (index: number) => void;
};

export function SectionFieldGrid({
  fieldsPath,
  sectionName,
  builtinSection,
  onOpenAddField,
  onEditField,
}: SectionFieldGridProps) {
  const { t } = useTranslation();
  const form = useFormContext<FormSchemaType>();
  const { fields, remove } = useFieldArray({
    control: form.control,
    name: fieldsPath,
  });

  const isTypedSection = sectionName === 'entity' || sectionName === 'relation';

  const availableTypes = useMemo(
    () => getTypeOptionsFromBuiltinSection(builtinSection),
    [builtinSection],
  );

  const currentFields = useWatch({
    control: form.control,
    name: fieldsPath,
  }) as Record<string, string>[] | undefined;

  const allTypesUsed = useMemo(() => {
    if (!isTypedSection || availableTypes.length === 0) return false;
    const availableSet = new Set(availableTypes.map((type) => type.value));
    const usedAvailableTypes = new Set(
      (currentFields ?? [])
        .map((field) => field.type)
        .filter(
          (type): type is string => Boolean(type) && availableSet.has(type),
        ),
    );
    return usedAvailableTypes.size >= availableTypes.length;
  }, [isTypedSection, availableTypes, currentFields]);

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

      {!allTypesUsed && (
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
      )}
    </section>
  );
}
