import { FormLayout } from '@/constants/form';
import { useTranslate } from '@/hooks/common-hooks';
import { SliderInputFormField } from './slider-input-form-field';

export function PageRankFormField() {
  const { t } = useTranslate('knowledgeConfiguration');

  return (
    <SliderInputFormField
      name={'pagerank'}
      label={t('pageRank')}
      tooltip={t('pageRankTip')}
      defaultValue={0}
      max={100}
      min={0}
      layout={FormLayout.Horizontal}
    ></SliderInputFormField>
  );
}

export default PageRankFormField;
