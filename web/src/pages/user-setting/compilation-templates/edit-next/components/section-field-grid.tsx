/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { Plus } from 'lucide-react';
import { useFieldArray, useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { SectionCardFieldMap } from '../constant';
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

  const cardFields = SectionCardFieldMap[sectionName];

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
            index={index}
            title={cardFields ? fieldValue[cardFields.title] : undefined}
            description={
              cardFields ? fieldValue[cardFields.description] : undefined
            }
            onEdit={onEditField}
            onDelete={remove}
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
          <span className="text-sm font-medium">{t('knowledgeCompilation.addField')}</span>
        </CardContent>
      </Card>
    </section>
  );
}
