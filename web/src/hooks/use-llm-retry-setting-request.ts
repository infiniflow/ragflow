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

import { getLlmMaxRetries, setLlmMaxRetries } from '@/services/admin-service';
import notification from '@/utils/notification';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

export const LlmRetrySettingKeys = {
  all: () => ['admin/llmMaxRetries'] as const,
};

/**
 * Reads the administrator's llm.max_retries override. `null` means no override
 * is saved, so the deployment's configured default applies.
 */
export const useFetchLlmMaxRetries = () =>
  useQuery({
    queryKey: LlmRetrySettingKeys.all(),
    queryFn: async () => {
      const { data } = await getLlmMaxRetries();
      if (data.code !== 0) {
        throw new Error(data.message);
      }
      const value = data.data?.[0]?.value?.trim() ?? '';
      return value === '' ? null : Number(value);
    },
    gcTime: 0,
  });

/** Saves the override; `null` clears it. */
export const useSaveLlmMaxRetries = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  return useMutation({
    mutationKey: ['admin/saveLlmMaxRetries'],
    mutationFn: async (value: number | null) => {
      const { data } = await setLlmMaxRetries(value);
      if (data.code !== 0) {
        throw new Error(data.message);
      }
      return value;
    },
    onSuccess: () => {
      notification.success({ message: t('admin.modelSettingsPage.saved') });
      return queryClient.invalidateQueries({
        queryKey: LlmRetrySettingKeys.all(),
      });
    },
    onError: (error: Error) => {
      notification.error({
        message: t('admin.modelSettingsPage.saveFailed'),
        description: error.message,
      });
    },
  });
};
