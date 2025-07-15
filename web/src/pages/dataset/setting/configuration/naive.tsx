import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { DelimiterFormField } from '@/components/delimiter-form-field';
import { ExcelToHtmlFormField } from '@/components/excel-to-html-form-field';
import { LayoutRecognizeFormField } from '@/components/layout-recognize-form-field';
import { MaxTokenNumberFormField } from '@/components/max-token-number-from-field';
import PageRankFormField from '@/components/page-rank-form-field';
import GraphRagItems from '@/components/parse-configuration/graph-rag-form-fields';
import RaptorFormFields from '@/components/parse-configuration/raptor-form-fields';
import {
  ConfigurationFormContainer,
  MainContainer,
} from '../configuration-form-container';
import { TagItems } from '../tag-item';
import { ChunkMethodItem, EmbeddingModelItem } from './common-item';

export function NaiveConfiguration() {
  return (
    <MainContainer>
      <ConfigurationFormContainer>
        <ChunkMethodItem></ChunkMethodItem>
        <LayoutRecognizeFormField></LayoutRecognizeFormField>
        <EmbeddingModelItem></EmbeddingModelItem>
        <MaxTokenNumberFormField initialValue={512}></MaxTokenNumberFormField>
        <DelimiterFormField></DelimiterFormField>
      </ConfigurationFormContainer>
      <ConfigurationFormContainer>
        <PageRankFormField></PageRankFormField>
        <AutoKeywordsFormField></AutoKeywordsFormField>
        <AutoQuestionsFormField></AutoQuestionsFormField>
        <ExcelToHtmlFormField></ExcelToHtmlFormField>
        <TagItems></TagItems>
      </ConfigurationFormContainer>
      <ConfigurationFormContainer>
        <RaptorFormFields></RaptorFormFields>
      </ConfigurationFormContainer>
      <GraphRagItems></GraphRagItems>
    </MainContainer>
  );
}
