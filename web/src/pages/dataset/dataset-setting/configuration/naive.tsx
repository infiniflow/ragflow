import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { DelimiterFormField } from '@/components/delimiter-form-field';
import { ExcelToHtmlFormField } from '@/components/excel-to-html-form-field';
import { LayoutRecognizeFormField } from '@/components/layout-recognize-form-field';
import { MaxTokenNumberFormField } from '@/components/max-token-number-from-field';
import { MinerUOptionsFormField } from '@/components/mineru-options-form-field';
import {
  ConfigurationFormContainer,
  MainContainer,
} from '../configuration-form-container';
import { EnableTocToggle, OverlappedPercent } from './common-item';

export function NaiveConfiguration() {
  return (
    <MainContainer>
      <ConfigurationFormContainer>
        <LayoutRecognizeFormField></LayoutRecognizeFormField>
        <MinerUOptionsFormField></MinerUOptionsFormField>
        <MaxTokenNumberFormField initialValue={512}></MaxTokenNumberFormField>
        <DelimiterFormField></DelimiterFormField>
        <EnableTocToggle />
        <OverlappedPercent />
      </ConfigurationFormContainer>
      <ConfigurationFormContainer>
        <AutoKeywordsFormField></AutoKeywordsFormField>
        <AutoQuestionsFormField></AutoQuestionsFormField>
        <ExcelToHtmlFormField></ExcelToHtmlFormField>
        {/* <TagItems></TagItems> */}
      </ConfigurationFormContainer>
    </MainContainer>
  );
}
