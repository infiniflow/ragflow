import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { ConfigurationFormContainer } from '../configuration-form-container';
import { AutoMetadata, GlobalIndexModelItem } from './common-item';
import { FormLayout } from '@/constants/form';

export function PictureConfiguration() {
  return (
    <ConfigurationFormContainer>
      <>
        <GlobalIndexModelItem />
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
