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

import { Collapse } from '@/components/collapse';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SliderInputFormField } from '@/components/slider-input-form-field';
import { SwitchFormField } from '@/components/switch-fom-field';
import { Textarea } from '@/components/ui/textarea';
import { useTranslation } from 'react-i18next';

type TreeTemplateFieldsProps = {
  index: number;
};

export function TreeTemplateFields({ index }: TreeTemplateFieldsProps) {
  const { t } = useTranslation();

  return (
    <Collapse defaultOpen title={t('knowledgeCompilation.raptorTreeSettings')}>
      <div className="space-y-4">
        <RAGFlowFormItem
          name={`templates.${index}.config.raptor.prompt`}
          label={t('knowledgeCompilation.summarizationPrompt')}
        >
          <Textarea
            placeholder={t('common.descriptionPlaceholder')}
            rows={8}
            resize="vertical"
          />
        </RAGFlowFormItem>

        <SliderInputFormField
          name={`templates.${index}.config.raptor.max_token`}
          label={t('knowledgeCompilation.maxToken')}
          max={2048}
          min={512}
          step={1}
        />
        <SliderInputFormField
          name={`templates.${index}.config.raptor.clustering_threshold`}
          label={t('knowledgeCompilation.clusteringThreshold')}
          tooltip={t('knowledgeCompilation.clusteringThresholdTip')}
          step={0.01}
          max={1}
          min={0}
        />
        <SliderInputFormField
          name={`templates.${index}.config.raptor.clustering_ratio`}
          label={t('knowledgeCompilation.clusteringRatio')}
          tooltip={t('knowledgeCompilation.clusteringRatioTip')}
          step={0.01}
          max={1}
          min={0}
        />

        <SwitchFormField
          name={`templates.${index}.config.raptor.rechunk`}
          label={t('knowledgeCompilation.rechunkByTreeLeaves')}
          tooltip={t('knowledgeCompilation.rechunkByTreeLeavesTip')}
          vertical={false}
        />
      </div>
    </Collapse>
  );
}
