import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { DelimiterFormField } from '@/components/delimiter-form-field';
import { ExcelToHtmlFormField } from '@/components/excel-to-html-form-field';
import { FormContainer } from '@/components/form-container';
import { LayoutRecognizeFormField } from '@/components/layout-recognize-form-field';
import { MaxTokenNumberFormField } from '@/components/max-token-number-from-field';
import PageRankFormField from '@/components/page-rank-form-field';
import GraphRagItems from '@/components/parse-configuration/graph-rag-form-fields';
import RaptorFormFields from '@/components/parse-configuration/raptor-form-fields';
import { TagItems } from '../tag-item';
import { ChunkMethodItem, EmbeddingModelItem } from './common-item';

export function NaiveConfiguration() {
  return (
    <section className="space-y-5 mb-4 overflow-auto">
      <FormContainer>
        <LayoutRecognizeFormField></LayoutRecognizeFormField>
        <EmbeddingModelItem></EmbeddingModelItem>
        <ChunkMethodItem></ChunkMethodItem>
        <MaxTokenNumberFormField></MaxTokenNumberFormField>
        <DelimiterFormField></DelimiterFormField>
      </FormContainer>
      <FormContainer>
        <PageRankFormField></PageRankFormField>
        <AutoKeywordsFormField></AutoKeywordsFormField>
        <AutoQuestionsFormField></AutoQuestionsFormField>
        <ExcelToHtmlFormField></ExcelToHtmlFormField>
        <TagItems></TagItems>
      </FormContainer>
      <FormContainer>
        <RaptorFormFields></RaptorFormFields>
      </FormContainer>
      <GraphRagItems></GraphRagItems>
    </section>
  );
}
