import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { LayoutRecognizeFormField } from '@/components/layout-recognize-form-field';
import { ConfigurationFormContainer } from '../configuration-form-container';
import { useOwnerTenantId } from '../../../contexts/knowledge-base-context';
import { AutoMetadata, GlobalIndexModelItem } from './common-item';
import { FormLayout } from '@/constants/form';

export function OneConfiguration() {
  const ownerTenantId = useOwnerTenantId();
  return (
    <ConfigurationFormContainer>
      <LayoutRecognizeFormField
        ownerTenantId={ownerTenantId}
      ></LayoutRecognizeFormField>
      <GlobalIndexModelItem />
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
