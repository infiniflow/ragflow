import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { ConfigurationFormContainer } from '../configuration-form-container';

export function PictureConfiguration() {
  return (
    <ConfigurationFormContainer>
      <>
        <AutoKeywordsFormField></AutoKeywordsFormField>
        <AutoQuestionsFormField></AutoQuestionsFormField>
      </>
      {/* <TagItems></TagItems> */}
    </ConfigurationFormContainer>
  );
}
