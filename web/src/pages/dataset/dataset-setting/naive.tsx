import GraphRagItems from '@/components/parse-configuration/graph-rag-form-fields';
import RaptorFormFields from '@/components/parse-configuration/raptor-form-fields';
import {
  ConfigurationFormContainer,
  MainContainer,
} from './configuration-form-container';
import { EnableAutoGenerateItem } from './configuration/common-item';

export function NaiveConfiguration() {
  return (
    <MainContainer>
      <GraphRagItems className="border-none p-0"></GraphRagItems>
      <ConfigurationFormContainer>
        <RaptorFormFields></RaptorFormFields>
      </ConfigurationFormContainer>
      <EnableAutoGenerateItem />
      {/* <ConfigurationFormContainer>
        <ChunkMethodItem></ChunkMethodItem>
        <LayoutRecognizeFormField></LayoutRecognizeFormField>

        <MaxTokenNumberFormField initialValue={512}></MaxTokenNumberFormField>
        <DelimiterFormField></DelimiterFormField>
      </ConfigurationFormContainer>
      <ConfigurationFormContainer>
        <PageRankFormField></PageRankFormField>
        <AutoKeywordsFormField></AutoKeywordsFormField>
        <AutoQuestionsFormField></AutoQuestionsFormField>
        <ExcelToHtmlFormField></ExcelToHtmlFormField>
        <TagItems></TagItems>
      </ConfigurationFormContainer> */}
    </MainContainer>
  );
}
