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

import { Checkbox } from '@/components/ui/checkbox';
import { useTranslate } from '@/hooks/common-hooks';
import { IProviderModelItem } from '@/interfaces/request/llm';
import { Pencil } from 'lucide-react';
import { useMemo } from 'react';

interface UseListModelsOptionsParams {
  models: IProviderModelItem[];
  selectedModelItems: IProviderModelItem[];
  allSelected: boolean;
  handleSelectModel: (model: IProviderModelItem) => void;
  handleToggleAll: () => void;
  onEditModel?: (model: IProviderModelItem) => void;
}

export const useListModelsOptions = ({
  models,
  selectedModelItems,
  allSelected,
  handleSelectModel,
  handleToggleAll,
  onEditModel,
}: UseListModelsOptionsParams) => {
  const { t } = useTranslate('setting');

  return useMemo(() => {
    const allOption = {
      value: null as string | null,
      label: (
        <div className="flex justify-between items-center gap-2 w-full">
          <div className="flex-1 min-w-0 flex gap-1 items-center">
            <div className="font-medium truncate">{t('allModels')}</div>
          </div>
          <Checkbox
            checked={allSelected}
            onClick={(e) => {
              e.stopPropagation();
              handleToggleAll();
            }}
          />
        </div>
      ),
      onClick: () => handleToggleAll(),
    };

    const modelOptions = models.map((m) => {
      const checked = selectedModelItems.some((s) => s.name === m.name);
      return {
        value: m.name,
        label: (
          <div className="flex justify-between items-center gap-2 w-full">
            <div className="flex-1 min-w-0 flex gap-1 items-center">
              <div className="font-medium truncate">{m.name}</div>
              {m.model_types &&
                m.model_types.map((type) => {
                  return (
                    <div
                      key={type}
                      className="text-xs text-text-secondary truncate bg-bg-card rounded-md px-2 py-1"
                    >
                      {type}
                    </div>
                  );
                })}
            </div>
            <div className="flex items-center gap-1">
              {onEditModel && (
                <button
                  type="button"
                  aria-label="Edit model"
                  title="Edit model"
                  className="p-1 rounded hover:bg-bg-card text-text-secondary"
                  onClick={(e) => {
                    e.stopPropagation();
                    onEditModel(m);
                  }}
                >
                  <Pencil size={12} />
                </button>
              )}
              <Checkbox
                checked={checked}
                onClick={(e) => {
                  e.stopPropagation();
                  handleSelectModel(m);
                }}
              />
            </div>
          </div>
        ),
        onClick: () => handleSelectModel(m),
      };
    });
    if (modelOptions?.length) {
      return [allOption, ...modelOptions];
    } else {
      return [];
    }
  }, [
    models,
    selectedModelItems,
    handleSelectModel,
    allSelected,
    handleToggleAll,
    onEditModel,
    t,
  ]);
};
