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

import { useTranslate } from '@/hooks/common-hooks';
import {
  canonicalTableColumnRole,
  TableColumnRole,
} from '@/utils/table-column-settings';
import { FormControl, FormItem, FormLabel } from './ui/form';
import { RadioGroup, RadioGroupItem } from './ui/radio-group';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from './ui/select';

const RoleOptions: { value: TableColumnRole; labelKey: string }[] = [
  { value: 'both', labelKey: 'tableColumnRoleBoth' },
  { value: 'indexing', labelKey: 'tableColumnRoleIndexing' },
  { value: 'metadata', labelKey: 'tableColumnRoleMetadata' },
];

type TableColumnSettingsFieldsProps = {
  // The radio inputs are labelled by id, so each host passes a prefix that
  // keeps them unique when one page shows the settings more than once.
  idPrefix: string;
  // The persisted mode and roles, taken verbatim: only the exact "manual"
  // selects manual, and a stored role is canonicalised the way the resolver
  // reads it, so what a host shows is what it will save.
  mode: unknown;
  columns: unknown;
  roles: Record<string, string | undefined> | null | undefined;
  onModeChange: (mode: 'auto' | 'manual') => void;
  onRoleChange: (column: string, role: TableColumnRole) => void;
};

/**
 * The table column settings a reader configures: which mode the parse uses and,
 * under manual, the role every discovered column plays. Rendered by every host
 * that owns the values — the parser operator form, the dataset's table
 * configuration, the upload dialog — none of which share a form or a state
 * shape.
 */
export function TableColumnSettingsFields({
  idPrefix,
  mode: storedMode,
  columns: storedColumns,
  roles,
  onModeChange,
  onRoleChange,
}: TableColumnSettingsFieldsProps) {
  const { t } = useTranslate('knowledgeConfiguration');
  const mode = storedMode === 'manual' ? 'manual' : 'auto';
  const columns: string[] = Array.isArray(storedColumns) ? storedColumns : [];

  return (
    <>
      <FormItem className="space-y-2">
        <FormLabel className="text-sm font-medium">
          {t('tableColumnMode')}
        </FormLabel>
        <FormControl>
          <RadioGroup
            value={mode}
            onValueChange={onModeChange}
            className="flex gap-4"
          >
            <div className="flex items-center space-x-2">
              <RadioGroupItem
                value="auto"
                id={`${idPrefix}-column-mode-auto`}
              />
              <label
                htmlFor={`${idPrefix}-column-mode-auto`}
                className="text-sm font-normal cursor-pointer"
              >
                {t('tableColumnModeAuto')}
              </label>
            </div>
            <div className="flex items-center space-x-2">
              <RadioGroupItem
                value="manual"
                id={`${idPrefix}-column-mode-manual`}
              />
              <label
                htmlFor={`${idPrefix}-column-mode-manual`}
                className="text-sm font-normal cursor-pointer"
              >
                {t('tableColumnModeManual')}
              </label>
            </div>
          </RadioGroup>
        </FormControl>
      </FormItem>

      {mode === 'auto' && (
        <p className="text-sm text-muted-foreground">
          {t('tableColumnModeAutoDescription')}
        </p>
      )}

      {mode === 'manual' && columns.length === 0 && (
        <p className="text-sm text-muted-foreground">
          {t('tableColumnRolesEmpty')}
        </p>
      )}

      {mode === 'manual' && columns.length > 0 && (
        <>
          <p className="text-sm text-muted-foreground mb-3">
            {t('tableColumnRolesTip')}
          </p>
          <div className="space-y-3 max-h-[200px] overflow-y-auto">
            {columns.map((column) => (
              <FormItem
                key={column}
                className="flex flex-row items-center gap-4"
              >
                <FormLabel className="min-w-[120px] shrink-0 text-sm font-normal">
                  {column}
                </FormLabel>
                <FormControl>
                  <Select
                    value={canonicalTableColumnRole(roles?.[column])}
                    onValueChange={(value) =>
                      onRoleChange(column, value as TableColumnRole)
                    }
                  >
                    <SelectTrigger className="w-[160px]">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {RoleOptions.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {t(option.labelKey)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </FormControl>
              </FormItem>
            ))}
          </div>
          <p className="text-xs text-muted-foreground mt-3">
            {t('tableColumnRolesReparseTip')}
          </p>
        </>
      )}
    </>
  );
}
