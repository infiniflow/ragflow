import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import PageRankFormField from '@/components/page-rank-form-field';
import { ConfigurationFormContainer } from '../configuration-form-container';
import { TagItems } from '../tag-item';
import { ChunkMethodItem, EmbeddingModelItem } from './common-item';

export function PictureConfiguration() {
  return (
    <ConfigurationFormContainer>
      <ChunkMethodItem></ChunkMethodItem>
      <EmbeddingModelItem></EmbeddingModelItem>

      <PageRankFormField></PageRankFormField>

      <>
        <AutoKeywordsFormField></AutoKeywordsFormField>
        <AutoQuestionsFormField></AutoQuestionsFormField>
      </>
      <TagItems></TagItems>
    </ConfigurationFormContainer>
  );
}
