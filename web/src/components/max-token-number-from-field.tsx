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
