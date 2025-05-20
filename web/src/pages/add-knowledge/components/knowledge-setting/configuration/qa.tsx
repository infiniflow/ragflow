import PageRank from '@/components/page-rank';
import { TagItems } from '../tag-item';
import { ChunkMethodItem, EmbeddingModelItem } from './common-item';

export function QAConfiguration() {
  return (
    <>
      <EmbeddingModelItem></EmbeddingModelItem>
      <ChunkMethodItem></ChunkMethodItem>

      <PageRank></PageRank>

      <TagItems></TagItems>
    </>
  );
}
