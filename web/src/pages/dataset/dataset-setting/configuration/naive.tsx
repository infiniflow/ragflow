import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { ChildrenDelimiterForm } from '@/components/children-delimiter-form';
import { DelimiterFormField } from '@/components/delimiter-form-field';
import { ExcelToHtmlFormField } from '@/components/excel-to-html-form-field';
import { LayoutRecognizeFormField } from '@/components/layout-recognize-form-field';
import { MaxTokenNumberFormField } from '@/components/max-token-number-from-field';
import {
  ConfigurationFormContainer,
  MainContainer,
} from '../configuration-form-container';
import {
  AutoMetadata,
  EnableTocToggle,
  ImageContextWindow,
  OverlappedPercent,
} from './common-item';

export function NaiveConfiguration() {
  return (
    <MainContainer>
      <ConfigurationFormContainer>
        <LayoutRecognizeFormField></LayoutRecognizeFormField>
        <MaxTokenNumberFormField initialValue={512}></MaxTokenNumberFormField>
        <DelimiterFormField></DelimiterFormField>
        <ChildrenDelimiterForm />
        <EnableTocToggle />
        <ImageContextWindow />
        <AutoMetadata />
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
