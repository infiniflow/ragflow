import PageRankFormField from '@/components/page-rank-form-field';
import { TagItems } from '../tag-item';
import { ChunkMethodItem, EmbeddingModelItem } from './common-item';

export function QAConfiguration() {
  return (
    <>
      <EmbeddingModelItem></EmbeddingModelItem>
      <ChunkMethodItem></ChunkMethodItem>

      <PageRankFormField></PageRankFormField>

      <TagItems></TagItems>
    </>
  );
}
