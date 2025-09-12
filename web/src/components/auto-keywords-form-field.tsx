import { FormLayout } from '@/constants/form';
import { useTranslate } from '@/hooks/common-hooks';
import { SliderInputFormField } from './slider-input-form-field';

export function AutoKeywordsFormField() {
  const { t } = useTranslate('knowledgeDetails');

  return (
    <SliderInputFormField
      name={'parser_config.auto_keywords'}
      label={t('autoKeywords')}
      max={30}
      min={0}
      tooltip={t('autoKeywordsTip')}
      layout={FormLayout.Horizontal}
    ></SliderInputFormField>
  );
}

export function AutoQuestionsFormField() {
  const { t } = useTranslate('knowledgeDetails');

  return (
    <SliderInputFormField
      name={'parser_config.auto_questions'}
      label={t('autoQuestions')}
      max={10}
      min={0}
      tooltip={t('autoQuestionsTip')}
      layout={FormLayout.Horizontal}
    ></SliderInputFormField>
  );
}
