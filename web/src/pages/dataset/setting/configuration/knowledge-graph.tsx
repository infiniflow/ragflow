import { DelimiterFormField } from '@/components/delimiter-form-field';
import { EntityTypesFormField } from '@/components/entity-types-form-field';
import { MaxTokenNumberFormField } from '@/components/max-token-number-from-field';
import PageRankFormField from '@/components/page-rank-form-field';
import { ChunkMethodItem, EmbeddingModelItem } from './common-item';

export function KnowledgeGraphConfiguration() {
  return (
    <>
      <EmbeddingModelItem></EmbeddingModelItem>
      <ChunkMethodItem></ChunkMethodItem>

      <PageRankFormField></PageRankFormField>

      <>
        <EntityTypesFormField></EntityTypesFormField>
        <MaxTokenNumberFormField max={8192 * 2}></MaxTokenNumberFormField>
        <DelimiterFormField></DelimiterFormField>
      </>
    </>
  );
}
