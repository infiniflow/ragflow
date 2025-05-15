import PageRankFormField from '@/components/page-rank-form-field';
import { ChunkMethodItem, EmbeddingModelItem } from './common-item';

export function TagConfiguration() {
  return (
    <>
      <EmbeddingModelItem></EmbeddingModelItem>
      <ChunkMethodItem></ChunkMethodItem>

      <PageRankFormField></PageRankFormField>
    </>
  );
}
