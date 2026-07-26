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
    ></SliderInputFormField>
  );
}
