import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { LayoutRecognizeFormField } from '@/components/layout-recognize-form-field';
import PageRankFormField from '@/components/page-rank-form-field';
import GraphRagItems from '@/components/parse-configuration/graph-rag-form-fields';
import RaptorFormFields from '@/components/parse-configuration/raptor-form-fields';
import {
  ConfigurationFormContainer,
  MainContainer,
} from '../configuration-form-container';
import { TagItems } from '../tag-item';
import { ChunkMethodItem, EmbeddingModelItem } from './common-item';

export function BookConfiguration() {
  return (
    <MainContainer>
      <ConfigurationFormContainer>
        <ChunkMethodItem></ChunkMethodItem>
        <LayoutRecognizeFormField></LayoutRecognizeFormField>
        <EmbeddingModelItem></EmbeddingModelItem>

        <PageRankFormField></PageRankFormField>
      </ConfigurationFormContainer>

      <ConfigurationFormContainer>
        <AutoKeywordsFormField></AutoKeywordsFormField>
        <AutoQuestionsFormField></AutoQuestionsFormField>
      </ConfigurationFormContainer>

      <ConfigurationFormContainer>
        <RaptorFormFields></RaptorFormFields>
      </ConfigurationFormContainer>

      <GraphRagItems marginBottom className="p-10"></GraphRagItems>

      <ConfigurationFormContainer>
        <TagItems></TagItems>
      </ConfigurationFormContainer>
    </MainContainer>
  );
}
