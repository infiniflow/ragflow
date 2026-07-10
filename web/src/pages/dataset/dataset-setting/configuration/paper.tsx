import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { LayoutRecognizeFormField } from '@/components/layout-recognize-form-field';
import { useKnowledgeBaseContext } from '../../contexts/knowledge-base-context';
import {
  ConfigurationFormContainer,
  MainContainer,
} from '../configuration-form-container';
import { AutoMetadata } from './common-item';

export function PaperConfiguration() {
  const ownerTenantId = useKnowledgeBaseContext().knowledgeBase?.tenant_id;
  return (
    <MainContainer>
      <ConfigurationFormContainer>
        <LayoutRecognizeFormField
          ownerTenantId={ownerTenantId}
        ></LayoutRecognizeFormField>
      </ConfigurationFormContainer>

      <ConfigurationFormContainer>
        <AutoMetadata />
        <AutoKeywordsFormField></AutoKeywordsFormField>
        <AutoQuestionsFormField></AutoQuestionsFormField>
      </ConfigurationFormContainer>
      {/* <ConfigurationFormContainer>
        <TagItems></TagItems>
      </ConfigurationFormContainer> */}
    </MainContainer>
  );
}
