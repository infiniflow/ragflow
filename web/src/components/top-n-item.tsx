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

import { FormLayout } from '@/constants/form';
import { useTranslate } from '@/hooks/common-hooks';
import { z } from 'zod';
import { SliderInputFormField } from './slider-input-form-field';

interface SimilaritySliderFormFieldProps {
  max?: number;
  name?: string;
}

export const topnSchema = {
  top_n: z.number().optional(),
};

export function TopNFormField({
  max = 30,
  name = 'top_n',
}: SimilaritySliderFormFieldProps) {
  const { t } = useTranslate('chat');

  return (
    <SliderInputFormField
      name={name}
      label={t('topN')}
      max={max}
      tooltip={t('topNTip')}
      layout={FormLayout.Vertical}
    ></SliderInputFormField>
  );
}
