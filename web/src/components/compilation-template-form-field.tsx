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

import { RAGFlowFormItem } from '@/components/ragflow-form';
import { useCompilationTemplateGroupOptions } from '@/hooks/use-compilation-template-group-request';
import { ICompilationTemplateGroup } from '@/interfaces/database/compilation-template';
import { TriangleAlert } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { CreateCompilationTemplateGroupDialog } from './create-compilation-template-group-dialog';
import { SelectWithSearch } from './originui/select-with-search';

type CompilationTemplateFormFieldProps = {
  horizontal?: boolean;
  name?: string;
  ownerTenantId?: string;
};

export function CompilationTemplateFormField({
  horizontal,
  name = 'parser_config.compilation_template_group_id',
  ownerTenantId,
}: CompilationTemplateFormFieldProps) {
  const { t } = useTranslation();
  const { getValues, setValue } = useFormContext();
  const { options, isFetched, isError } =
    useCompilationTemplateGroupOptions(ownerTenantId);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);

  // Zero-arg wrapper: labelLink.onClick is wired to a DOM click, whose event
  // would be read as the updater value if setCreateDialogOpen were passed
  // directly.
  const openCreateDialog = useCallback(() => {
    setCreateDialogOpen(true);
  }, []);

  // Adopt the freshly created group only when the field is still empty — an
  // existing selection is the user's and must survive template creation.
  const adoptCreatedGroup = useCallback(
    (group: ICompilationTemplateGroup) => {
      if (!getValues(name)) {
        setValue(name, group.id, { shouldDirty: true, shouldValidate: true });
      }
    },
    [getValues, name, setValue],
  );

  // No owner-scoped name lookup exists for template groups (strictly
  // owner-tenant, no sharing mechanism) — the raw id is all the missing
  // display can show. Styled like MissingModelLabel.
  const renderMissingGroup = useCallback(
    (missingValue: string) => (
      <span className="flex items-center gap-1.5 text-text-disabled">
        <TriangleAlert className="size-4 flex-shrink-0" />
        <span className="truncate">{missingValue}</span>
      </span>
    ),
    [],
  );

  return (
    <>
      <RAGFlowFormItem
        name={name}
        label={t('knowledgeConfiguration.compilationTemplate')}
        labelLink={{
          text: t('knowledgeConfiguration.createTemplate'),
          onClick: openCreateDialog,
        }}
        className="pb-4"
        horizontal={horizontal}
        required
      >
        {(field) => (
          <SelectWithSearch
            value={field.value}
            onChange={field.onChange}
            options={options}
            loading={!isFetched || isError}
            renderMissingValue={renderMissingGroup}
          />
        )}
      </RAGFlowFormItem>
      <CreateCompilationTemplateGroupDialog
        open={createDialogOpen}
        onOpenChange={setCreateDialogOpen}
        onCreated={adoptCreatedGroup}
      />
    </>
  );
}
