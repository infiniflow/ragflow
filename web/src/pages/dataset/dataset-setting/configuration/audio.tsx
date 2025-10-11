import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { ConfigurationFormContainer } from '../configuration-form-container';

import { TagItems } from '../components/tag-item';

export function AudioConfiguration() {
  return (
    <ConfigurationFormContainer>
      <>
        <AutoKeywordsFormField></AutoKeywordsFormField>
        <AutoQuestionsFormField></AutoQuestionsFormField>
      </>

      <TagItems></TagItems>
    </ConfigurationFormContainer>
  );
}
