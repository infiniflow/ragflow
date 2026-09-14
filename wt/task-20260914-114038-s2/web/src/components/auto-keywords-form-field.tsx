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

interface AutoFieldProps {
  name?: string;
  layout?: FormLayout;
}

export function AutoKeywordsFormField({
  name = 'parser_config.auto_keywords',
  layout = FormLayout.Vertical,
}: AutoFieldProps) {
  const { t } = useTranslate('knowledgeDetails');

  return (
    <SliderInputFormField
      name={name}
      label={t('autoKeywords')}
      max={30}
      min={0}
      tooltip={t('autoKeywordsTip')}
      layout={layout}
      sliderTestId="ds-settings-parser-auto-keyword-slider"
      numberInputTestId="ds-settings-parser-auto-keyword-input"
      integer
    ></SliderInputFormField>
  );
}

export function AutoQuestionsFormField({
  name = 'parser_config.auto_questions',
  layout = FormLayout.Vertical,
}: AutoFieldProps) {
  const { t } = useTranslate('knowledgeDetails');

  return (
    <SliderInputFormField
      name={name}
      label={t('autoQuestions')}
      max={10}
      min={0}
      tooltip={t('autoQuestionsTip')}
      layout={layout}
      sliderTestId="ds-settings-parser-auto-question-slider"
      numberInputTestId="ds-settings-parser-auto-question-input"
      integer
    ></SliderInputFormField>
  );
}
