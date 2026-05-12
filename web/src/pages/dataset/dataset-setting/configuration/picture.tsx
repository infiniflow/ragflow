import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { LayoutRecognizeFormField } from '@/components/layout-recognize-form-field';
import { ConfigurationFormContainer } from '../configuration-form-container';
import { AutoMetadata } from './common-item';

export function PictureConfiguration() {
  return (
    <ConfigurationFormContainer>
      <>
        <LayoutRecognizeFormField
          optionsWithoutLLM={[{ value: '', label: 'Built-in OCR' }]}
          showMineruOptions={false}
          showPaddleocrOptions={true}
        />
        <AutoMetadata />
        <AutoKeywordsFormField></AutoKeywordsFormField>
        <AutoQuestionsFormField></AutoQuestionsFormField>
      </>
    </ConfigurationFormContainer>
  );
}
