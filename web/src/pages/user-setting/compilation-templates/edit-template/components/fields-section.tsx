import { Collapse } from '@/components/collapse';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { startCase } from 'lodash';
import { Plus, Trash2 } from 'lucide-react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { Textarea } from '@/components/ui/textarea';

import { FormSchemaType } from '../schema';
import { createEmptyField, FieldLabelKeyMap } from '../utils';

type FieldsSectionProps = {
  name: `config.${string}`;
  title: string;
  fieldKeys: string[];
  defaultOpen?: boolean;
  typeOptions: { label: string; value: string }[];
};

export function FieldsSection({
  name,
  title,
  fieldKeys,
  defaultOpen = false,
  typeOptions,
}: FieldsSectionProps) {
  const { t } = useTranslation();
  const form = useFormContext<FormSchemaType>();
  const fieldsPath = `${name}.fields` as const;

  const { fields, append, remove } = useFieldArray({
    name: fieldsPath,
    control: form.control,
  });

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
                  <RAGFlowFormItem
                    key={key}
                    name={`${fieldsPath}.${index}.${key}`}
                    label={
                      FieldLabelKeyMap[key]
                        ? t(FieldLabelKeyMap[key])
                        : startCase(key)
                    }
                    required
                  >
                    {key === 'type' ? (
                      <SelectWithSearch
                        options={typeOptions}
                        placeholder={t('common.selectPlaceholder')}
                      />
                    ) : (
                      <Textarea
                        placeholder={t('setting.descriptionPlaceholder')}
                        rows={2}
                      />
                    )}
                  </RAGFlowFormItem>
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
