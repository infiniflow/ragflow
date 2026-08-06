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
    <Collapse defaultOpen title={t('setting.raptorTreeSettings')}>
      <div className="space-y-4">
        <RAGFlowFormItem
          name={`templates.${index}.config.raptor.prompt`}
          label={t('setting.summarizationPrompt')}
        >
          <Textarea
            placeholder={t('setting.descriptionPlaceholder')}
            rows={8}
            resize="vertical"
          />
        </RAGFlowFormItem>

        <SliderInputFormField
          name={`templates.${index}.config.raptor.max_token`}
          label={t('setting.maxToken')}
          max={2048}
          min={512}
          step={1}
        />
        <SliderInputFormField
          name={`templates.${index}.config.raptor.clustering_threshold`}
          label={t('setting.clusteringThreshold')}
          tooltip={t('setting.clusteringThresholdTip')}
          step={0.01}
          max={1}
          min={0}
        />
        <SliderInputFormField
          name={`templates.${index}.config.raptor.clustering_ratio`}
          label={t('setting.clusteringRatio')}
          tooltip={t('setting.clusteringRatioTip')}
          step={0.01}
          max={1}
          min={0}
        />

        <SwitchFormField
          name={`templates.${index}.config.raptor.rechunk`}
          label={t('setting.rechunkByTreeLeaves')}
          tooltip={t('setting.rechunkByTreeLeavesTip')}
          vertical={false}
        />
      </div>
    </Collapse>
  );
}
