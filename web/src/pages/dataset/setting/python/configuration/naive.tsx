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
import { useOwnerTenantId } from '../../../contexts/knowledge-base-context';
import {
  AutoMetadata,
  GlobalIndexModelItem,
  ImageContextWindow,
  OverlappedPercent,
} from './common-item';
import { FormLayout } from '@/constants/form';

export function NaiveConfiguration() {
  const ownerTenantId = useOwnerTenantId();
  return (
    <MainContainer>
      <ConfigurationFormContainer>
        <LayoutRecognizeFormField
          testId="ds-settings-parser-pdf-parser-select"
          ownerTenantId={ownerTenantId}
        ></LayoutRecognizeFormField>
        <GlobalIndexModelItem />
        <MaxTokenNumberFormField
          initialValue={512}
          sliderTestId="ds-settings-parser-recommended-chunk-size-slider"
          numberInputTestId="ds-settings-parser-recommended-chunk-size-input"
        ></MaxTokenNumberFormField>
        <OverlappedPercent />
        <DelimiterFormField></DelimiterFormField>
        <ChildrenDelimiterForm />
        <ImageContextWindow />
        <AutoMetadata />
      </ConfigurationFormContainer>
      <ConfigurationFormContainer>
        <AutoKeywordsFormField
          layout={FormLayout.Horizontal}
        ></AutoKeywordsFormField>
        <AutoQuestionsFormField
          layout={FormLayout.Horizontal}
        ></AutoQuestionsFormField>
        <ExcelToHtmlFormField></ExcelToHtmlFormField>
        {/* <TagItems></TagItems> */}
      </ConfigurationFormContainer>
    </MainContainer>
  );
}
