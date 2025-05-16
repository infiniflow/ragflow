import PageRankFormField from '@/components/page-rank-form-field';
import { ConfigurationFormContainer } from '../configuration-form-container';
import { ChunkMethodItem, EmbeddingModelItem } from './common-item';

export function TagConfiguration() {
  return (
    <ConfigurationFormContainer>
      <ChunkMethodItem></ChunkMethodItem>
      <EmbeddingModelItem></EmbeddingModelItem>

      <PageRankFormField></PageRankFormField>
    </ConfigurationFormContainer>
  );
}
