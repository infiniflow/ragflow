import { Collapse } from '@/components/collapse';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Textarea } from '@/components/ui/textarea';
import { ICompilationTemplateSection } from '@/interfaces/database/compilation-template';
import { startCase } from 'lodash';
import { Plus, Trash2 } from 'lucide-react';
import { useMemo } from 'react';
import {
  ArrayPath,
  Path,
  useFieldArray,
  useFormContext,
} from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { useFieldTypeChange } from '../hooks/use-field-type-change';
import { FormSchemaType } from '../schema';
import { createEmptyField, FieldLabelKeyMap } from '../utils';

type FieldsSectionProps = {
  name: Path<FormSchemaType>;
  title: string;
  fieldKeys: string[];
  defaultOpen?: boolean;
  builtinSection?: ICompilationTemplateSection;
};

export function FieldsSection({
  name,
  title,
  fieldKeys,
  defaultOpen = false,
  builtinSection,
}: FieldsSectionProps) {
  const { t } = useTranslation();
  const form = useFormContext<FormSchemaType>();
  const fieldsPath = `${name}.fields` as ArrayPath<FormSchemaType>;

  const { fields, append, remove } = useFieldArray({
    name: fieldsPath,
    control: form.control,
  });

  const typeOptions = useMemo(() => {
    const typeSet = new Set<string>();
    builtinSection?.fields?.forEach((field) => {
      if (field.type) typeSet.add(field.type);
    });
    return Array.from(typeSet)
      .sort()
      .map((value) => ({ label: value, value }));
  }, [builtinSection]);

  return (
    <Collapse defaultOpen={defaultOpen} title={title}>
      <section className="space-y-4">
        <RAGFlowFormItem
          name={`${name}.description`}
          label={t('setting.description')}
        >
          <Textarea
            placeholder={t('setting.descriptionPlaceholder')}
            rows={3}
          />
        </RAGFlowFormItem>

        <section className="space-y-3">
          {fields.map((field, index) => (
            <Card
              key={field.id}
              className="border-border-button bg-transparent transition-colors has-[button:hover]:border-state-error"
            >
              <CardContent className="p-4 space-y-4">
                <div className="flex items-center justify-between">
                  <span className="text-sm text-text-secondary">
                    {t('setting.field')} #{index + 1}
                  </span>
                  {fields.length > 1 && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-xs"
                      onClick={() => remove(index)}
                      className="text-text-secondary hover:text-state-error"
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  )}
                </div>

                {fieldKeys.map((key) => (
                  <FieldFormItem
                    key={key}
                    fieldKey={key}
                    fieldName={`${fieldsPath}.${index}.${key}`}
                    typeOptions={typeOptions}
                    builtinSection={builtinSection}
                    fieldsPath={fieldsPath}
                    index={index}
                  />
                ))}
              </CardContent>
            </Card>
          ))}
        </section>

        <Button
          type="button"
          variant="outline"
          onClick={() => append(createEmptyField(fieldKeys))}
        >
          <Plus className="size-4 mr-2" />
          {t('setting.addField')}
        </Button>
      </section>
    </Collapse>
  );
}

type FieldFormItemProps = {
  fieldKey: string;
  fieldName: string;
  typeOptions: { label: string; value: string }[];
  builtinSection?: ICompilationTemplateSection;
  fieldsPath: ArrayPath<FormSchemaType>;
  index: number;
};

function FieldFormItem({
  fieldKey,
  fieldName,
  typeOptions,
  builtinSection,
  fieldsPath,
  index,
}: FieldFormItemProps) {
  const { t } = useTranslation();
  const form = useFormContext<FormSchemaType>();
  const handleTypeChange = useFieldTypeChange({
    form,
    builtinSection,
    fieldsPath: fieldsPath as `templates.${number}.config.${string}.fields`,
    index,
  });

  return (
    <RAGFlowFormItem
      name={fieldName}
      label={
        FieldLabelKeyMap[fieldKey]
          ? t(FieldLabelKeyMap[fieldKey])
          : startCase(fieldKey)
      }
      required
    >
      {fieldKey === 'type' ? (
        (field) => (
          <SelectWithSearch
            options={typeOptions}
            value={field.value}
            onChange={(value) => handleTypeChange(field, value)}
            disabled={field.disabled}
            placeholder={t('common.selectPlaceholder')}
          />
        )
      ) : (
        <Textarea placeholder={t('setting.descriptionPlaceholder')} rows={2} />
      )}
    </RAGFlowFormItem>
  );
}
