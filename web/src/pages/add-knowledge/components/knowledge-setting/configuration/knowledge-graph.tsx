import Delimiter from '@/components/delimiter';
import EntityTypesItem from '@/components/entity-types-item';
import MaxTokenNumber from '@/components/max-token-number';
import PageRank from '@/components/page-rank';
import { ChunkMethodItem, EmbeddingModelItem } from './common-item';

export function KnowledgeGraphConfiguration() {
  return (
    <>
      <EmbeddingModelItem></EmbeddingModelItem>
      <ChunkMethodItem></ChunkMethodItem>

      <PageRank></PageRank>

      <>
        <EntityTypesItem></EntityTypesItem>
        <MaxTokenNumber max={8192 * 2}></MaxTokenNumber>
        <Delimiter></Delimiter>
      </>
    </>
  );
}
