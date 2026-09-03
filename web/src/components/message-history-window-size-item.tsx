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

import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import NumberInput from './originui/number-input';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from './ui/form';

type MessageHistoryWindowSizeFormFieldProps = {
  min?: number;
};
export function MessageHistoryWindowSizeFormField({
  min,
}: MessageHistoryWindowSizeFormFieldProps) {
  const form = useFormContext();
  const { t } = useTranslation();

  return (
    <FormField
      control={form.control}
      name={'message_history_window_size'}
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('flow.messageHistoryWindowSizeTip')}>
            {t('flow.messageHistoryWindowSize')}
          </FormLabel>
          <FormControl>
            <NumberInput {...field} min={min} className="w-full"></NumberInput>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
