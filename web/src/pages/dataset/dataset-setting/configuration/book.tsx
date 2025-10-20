import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { LayoutRecognizeFormField } from '@/components/layout-recognize-form-field';
import {
  ConfigurationFormContainer,
  MainContainer,
} from '../configuration-form-container';

export function BookConfiguration() {
  return (
    <MainContainer>
      <ConfigurationFormContainer>
        <LayoutRecognizeFormField></LayoutRecognizeFormField>
      </ConfigurationFormContainer>

      <ConfigurationFormContainer>
        <AutoKeywordsFormField></AutoKeywordsFormField>
        <AutoQuestionsFormField></AutoQuestionsFormField>
      </ConfigurationFormContainer>
      {/* <ConfigurationFormContainer>
        <TagItems></TagItems>
      </ConfigurationFormContainer> */}
    </MainContainer>
  );
}
