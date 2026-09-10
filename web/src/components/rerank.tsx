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

import { ModelTreeSelect } from '@/components/model-tree-select';
import { useTranslate } from '@/hooks/common-hooks';
import { prefixName } from '@/utils/form';
import { useFormContext } from 'react-hook-form';
import { z } from 'zod';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from './ui/form';

const DefaultRerankId = 'rerank_id';

interface RerankFormFieldProps {
  name?: string;
  ownerTenantId?: string;
  required?: boolean;
}

function RerankFormField({
  name = DefaultRerankId,
  ownerTenantId,
  required = false,
}: RerankFormFieldProps) {
  const form = useFormContext();
  const { t } = useTranslate('knowledgeDetails');

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('rerankTip')} required={required}>
            {t('rerankModel')}
          </FormLabel>
          <FormControl>
            <ModelTreeSelect
              modelTypes={['rerank']}
              allowClear
              placeholder={t('rerankPlaceholder')}
              ownerTenantId={ownerTenantId}
              {...field}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

export const rerankFormSchema = {
  [DefaultRerankId]: z.string().optional(),
};

interface RerankFormFieldsProps {
  prefix?: string;
  ownerTenantId?: string;
  required?: boolean;
}

export function RerankFormFields({
  prefix = '',
  ownerTenantId,
  required = false,
}: RerankFormFieldsProps) {
  const rerankIdName = prefixName(prefix, DefaultRerankId);

  return (
    <RerankFormField
      name={rerankIdName}
      ownerTenantId={ownerTenantId}
      required={required}
    ></RerankFormField>
  );
}
