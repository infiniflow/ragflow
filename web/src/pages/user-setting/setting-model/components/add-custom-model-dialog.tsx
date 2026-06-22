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

import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { MultiSelect } from '@/components/ui/multi-select';
import { useTranslate } from '@/hooks/common-hooks';
import { IProviderModelItem } from '@/interfaces/request/llm';
import { useEffect, useMemo, useState } from 'react';
import { mapModelKey } from './un-add-model';

interface AddCustomModelDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Existing model names; used for client-side uniqueness validation. */
  existingNames: string[];
  /**
   * Called when the user submits the form. The returned model is appended
   * to the parent component's local list (no API call from this dialog —
   * the parent decides whether to persist the model).
   */
  onSubmit: (item: IProviderModelItem) => void | Promise<void>;
  loading?: boolean;
}

/**
 * Simplified custom-model dialog for the v2 inline Models section.
 *
 * Differences from the v2-modal variant:
 *  - Fields are rendered directly here (no DynamicForm round-trip), so the
 *    caller can drop the dialog into an inline card without dragging in
 *    the dynamic-form context.
 *  - `model_types` is a multi-select driven by the keys of
 *    `mapModelKey` (single source of truth for human-readable tags).
 *  - No `features` switch-group; the v2-inline flow does not yet surface
 *    tool-call toggle.
 */
export function AddCustomModelDialog({
  open,
  onOpenChange,
  existingNames,
  onSubmit,
  loading = false,
}: AddCustomModelDialogProps) {
  const { t } = useTranslate('setting');

  const modelTypeOptions = useMemo(
    () =>
      Object.entries(mapModelKey).map(([value, label]) => ({
        label,
        value,
      })),
    [],
  );

  const [name, setName] = useState('');
  const [maxTokens, setMaxTokens] = useState<number>(0);
  const [modelTypes, setModelTypes] = useState<string[]>([]);
  const [nameError, setNameError] = useState<string | null>(null);

  // Reset the local form state whenever the dialog re-opens so the user
  // never sees stale values from a previous session.
  useEffect(() => {
    if (open) {
      setName('');
      setMaxTokens(0);
      setModelTypes([]);
      setNameError(null);
    }
  }, [open]);

  const handleSubmit = async () => {
    const trimmed = name.trim();
    if (!trimmed) {
      setNameError(t('modelNameRequired'));
      return;
    }
    if (existingNames.includes(trimmed)) {
      setNameError(t('modelNameDuplicate'));
      return;
    }
    await onSubmit({
      name: trimmed,
      max_tokens: maxTokens || 0,
      model_types: modelTypes,
      features: null,
    });
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-md"
        onClick={(e) => e.stopPropagation()}
        data-testid="add-custom-model-dialog"
      >
        <DialogHeader>
          <DialogTitle>{t('addCustomModelTitle')}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-1.5">
            <label className="text-sm font-normal text-text-secondary">
              {t('modelName')}
            </label>
            <Input
              value={name}
              onChange={(e) => {
                setName(e.target.value);
                if (nameError) setNameError(null);
              }}
              placeholder={t('modelName')}
              data-testid="add-custom-model-name"
            />
            {nameError && (
              <div className="text-xs text-state-error">{nameError}</div>
            )}
          </div>

          <div className="space-y-1.5">
            <label className="text-sm font-normal text-text-secondary">
              {t('modelMaxTokens')}
            </label>
            <Input
              type="number"
              value={maxTokens}
              onChange={(e) =>
                setMaxTokens(e.target.value === '' ? 0 : Number(e.target.value))
              }
              placeholder={t('modelMaxTokens')}
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-sm font-normal text-text-secondary">
              {t('modelType')}
            </label>
            <MultiSelect
              options={modelTypeOptions}
              value={modelTypes}
              onValueChange={setModelTypes}
              placeholder={t('modelType')}
            />
          </div>
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={loading}
          >
            {t('cancel')}
          </Button>
          <Button type="button" onClick={handleSubmit} disabled={loading}>
            {t('confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default AddCustomModelDialog;
