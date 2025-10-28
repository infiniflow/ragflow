import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { IncludeFormulasFormField } from '@/components/include-formulas-form-field';
import { LayoutRecognizeFormField } from '@/components/layout-recognize-form-field';
import { ConfigurationFormContainer } from '../configuration-form-container';

export function OneConfiguration() {
  return (
    <ConfigurationFormContainer>
      <LayoutRecognizeFormField></LayoutRecognizeFormField>
      <>
        <AutoKeywordsFormField></AutoKeywordsFormField>
        <AutoQuestionsFormField></AutoQuestionsFormField>
      </>
      <IncludeFormulasFormField></IncludeFormulasFormField>
      {/* <TagItems></TagItems> */}
    </ConfigurationFormContainer>
  );
}
