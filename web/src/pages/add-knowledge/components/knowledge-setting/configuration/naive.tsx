import {
  AutoKeywordsItem,
  AutoQuestionsItem,
} from '@/components/auto-keywords-item';
import Delimiter from '@/components/delimiter';
import ExcelToHtml from '@/components/excel-to-html';
import LayoutRecognize from '@/components/layout-recognize';
import MaxTokenNumber from '@/components/max-token-number';
import PageRank from '@/components/page-rank';
import ParseConfiguration from '@/components/parse-configuration';
import GraphRagItems from '@/components/parse-configuration/graph-rag-items';
import { TagItems } from '../tag-item';
import { ChunkMethodItem, EmbeddingModelItem } from './common-item';

export function NaiveConfiguration() {
  return (
    <>
      <EmbeddingModelItem></EmbeddingModelItem>
      <ChunkMethodItem></ChunkMethodItem>

      <>
        <AutoKeywordsItem></AutoKeywordsItem>
        <AutoQuestionsItem></AutoQuestionsItem>
      </>

      <>
        <MaxTokenNumber></MaxTokenNumber>
        <Delimiter></Delimiter>
        <LayoutRecognize></LayoutRecognize>
        <ExcelToHtml></ExcelToHtml>
      </>

      <ParseConfiguration></ParseConfiguration>

      <GraphRagItems></GraphRagItems>

      <TagItems></TagItems>
      <PageRank></PageRank>
    </>
  );
}
