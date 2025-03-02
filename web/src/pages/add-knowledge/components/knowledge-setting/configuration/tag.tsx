import PageRank from '@/components/page-rank';
import { ChunkMethodItem, EmbeddingModelItem } from './common-item';

export function TagConfiguration() {
  return (
    <>
      <EmbeddingModelItem></EmbeddingModelItem>
      <ChunkMethodItem></ChunkMethodItem>

      <PageRank></PageRank>
    </>
  );
}
