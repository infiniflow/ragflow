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
import { SliderInputFormField } from './slider-input-form-field';

interface IProps {
  initialValue?: number;
  max?: number;
  sliderTestId?: string;
  numberInputTestId?: string;
}

export function MaxTokenNumberFormField({
  max = 2048,
  initialValue,
  sliderTestId,
  numberInputTestId,
}: IProps) {
  const { t } = useTranslate('knowledgeConfiguration');

  return (
    <SliderInputFormField
      name={'parser_config.chunk_token_num'}
      label={t('chunkTokenNumber')}
      tooltip={t('chunkTokenNumberTip')}
      max={max}
      defaultValue={initialValue ?? 0}
      layout={FormLayout.Horizontal}
      sliderTestId={sliderTestId}
      numberInputTestId={numberInputTestId}
      min={1}
    ></SliderInputFormField>
  );
}
