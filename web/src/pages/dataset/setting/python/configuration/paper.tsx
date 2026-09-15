import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { LayoutRecognizeFormField } from '@/components/layout-recognize-form-field';
import {
  ConfigurationFormContainer,
  MainContainer,
} from '../configuration-form-container';
import { useOwnerTenantId } from '../../../contexts/knowledge-base-context';
import { AutoMetadata, GlobalIndexModelItem } from './common-item';
import { FormLayout } from '@/constants/form';

export function PaperConfiguration() {
  const ownerTenantId = useOwnerTenantId();
  return (
    <MainContainer>
      <ConfigurationFormContainer>
        <LayoutRecognizeFormField
          ownerTenantId={ownerTenantId}
        ></LayoutRecognizeFormField>
        <GlobalIndexModelItem />
      </ConfigurationFormContainer>

      <ConfigurationFormContainer>
        <AutoMetadata />
        <AutoKeywordsFormField
          layout={FormLayout.Horizontal}
        ></AutoKeywordsFormField>
        <AutoQuestionsFormField
          layout={FormLayout.Horizontal}
        ></AutoQuestionsFormField>
      </ConfigurationFormContainer>
      {/* <ConfigurationFormContainer>
        <TagItems></TagItems>
      </ConfigurationFormContainer> */}
    </MainContainer>
  );
}
