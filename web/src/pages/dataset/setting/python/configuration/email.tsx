import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { ConfigurationFormContainer } from '../configuration-form-container';
import { AutoMetadata } from './common-item';
import { FormLayout } from '@/constants/form';

export function EmailConfiguration() {
  return (
    <ConfigurationFormContainer>
      <>
        <AutoMetadata />
        <AutoKeywordsFormField
          layout={FormLayout.Horizontal}
        ></AutoKeywordsFormField>
        <AutoQuestionsFormField
          layout={FormLayout.Horizontal}
        ></AutoQuestionsFormField>
      </>
      {/* <TagItems></TagItems> */}
    </ConfigurationFormContainer>
  );
}
