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

import { ChangeEvent, FormEvent, useCallback, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { LucideLoader2, LucideSave } from 'lucide-react';

import RetryableError from '@/components/retryable-error';
import Spotlight from '@/components/spotlight';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  useFetchLlmMaxRetries,
  useSaveLlmMaxRetries,
} from '@/hooks/use-llm-retry-setting-request';

import {
  MaxLlmMaxRetries,
  MinLlmMaxRetries,
  parseLlmMaxRetriesInput,
} from './llm-max-retries';

const FieldId = 'llm-max-retries';

function AdminModelSettings() {
  const { t } = useTranslation();

  const {
    data: savedValue,
    isLoading,
    isFetching,
    isError,
    error: loadError,
    refetch,
  } = useFetchLlmMaxRetries();
  const { mutate: save, isPending: saving } = useSaveLlmMaxRetries();

  // Until the user edits the field it mirrors what is saved.
  const [draft, setDraft] = useState<string>();
  const input = draft ?? (savedValue == null ? '' : String(savedValue));
  const parsed = parseLlmMaxRetriesInput(input);
  const isDirty = parsed.valid && parsed.value !== (savedValue ?? null);

  // A number input reports text it cannot parse (such as "-") as "", which is
  // also what an intentionally blank field reports. The browser's own validity
  // state tells them apart, and the shared Input rebuilds its event target, so
  // it is read from the element.
  const inputRef = useRef<HTMLInputElement>(null);
  const [hasBadInput, setHasBadInput] = useState(false);

  // The shared number Input reports its value as a number, or "" when empty.
  const handleChange = useCallback((e: ChangeEvent<HTMLInputElement>) => {
    setHasBadInput(inputRef.current?.validity.badInput ?? false);
    setDraft(String(e.target.value));
  }, []);

  const canSave = !saving && !hasBadInput && parsed.valid && isDirty;

  const handleSubmit = useCallback(
    (e: FormEvent) => {
      e.preventDefault();
      if (hasBadInput || !parsed.valid) return;
      const submitted = input;
      // Keep whatever was typed while the save was running.
      save(parsed.value, {
        onSuccess: () =>
          setDraft((current) => (current === submitted ? undefined : current)),
      });
    },
    [hasBadInput, input, parsed, save],
  );

  const handleRetryLoad = useCallback(() => {
    refetch();
  }, [refetch]);

  const fieldError =
    hasBadInput || (!parsed.valid && parsed.reason === 'notInteger')
      ? t('admin.modelSettingsPage.errorNotInteger')
      : !parsed.valid
        ? t('admin.modelSettingsPage.errorOutOfRange', {
            min: MinLlmMaxRetries,
            max: MaxLlmMaxRetries,
          })
        : undefined;

  return (
    <Card className="!shadow-none relative h-full bg-transparent overflow-hidden">
      <Spotlight />

      <CardHeader className="space-y-0">
        <CardTitle className="leading-10">{t('admin.modelSettings')}</CardTitle>
        <CardDescription className="text-text-secondary">
          {t('admin.modelSettingsPage.description')}
        </CardDescription>
      </CardHeader>

      <CardContent>
        {isLoading ? (
          <LucideLoader2 className="size-5 animate-spin text-text-secondary" />
        ) : isError ? (
          <RetryableError
            message={loadError.message}
            onRetry={handleRetryLoad}
            retrying={isFetching}
          />
        ) : (
          <form
            className="max-w-xl space-y-4"
            onSubmit={handleSubmit}
            noValidate
          >
            <div className="space-y-2">
              <Label htmlFor={FieldId}>
                {t('admin.modelSettingsPage.maxRetries')}
              </Label>
              <Input
                ref={inputRef}
                id={FieldId}
                type="number"
                inputMode="numeric"
                min={MinLlmMaxRetries}
                max={MaxLlmMaxRetries}
                step={1}
                value={input}
                placeholder={t('admin.modelSettingsPage.useDefault')}
                aria-invalid={!!fieldError}
                aria-describedby={`${FieldId}-hint`}
                onChange={handleChange}
              />
              <p
                id={`${FieldId}-hint`}
                className={
                  fieldError
                    ? 'text-sm text-state-error'
                    : 'text-sm text-text-secondary'
                }
              >
                {fieldError ??
                  t('admin.modelSettingsPage.maxRetriesHint', {
                    min: MinLlmMaxRetries,
                    max: MaxLlmMaxRetries,
                  })}
              </p>
            </div>

            <Button type="submit" disabled={!canSave}>
              {saving ? (
                <LucideLoader2 className="animate-spin" />
              ) : (
                <LucideSave />
              )}
              {t('common.save')}
            </Button>
          </form>
        )}
      </CardContent>
    </Card>
  );
}

export default AdminModelSettings;
