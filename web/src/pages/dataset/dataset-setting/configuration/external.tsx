import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { ChildrenDelimiterForm } from '@/components/children-delimiter-form';
import { DelimiterFormField } from '@/components/delimiter-form-field';
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

// Same as NaiveConfiguration but without LayoutRecognize (parsing is done
// by the external loader) and without ExcelToHtml (input is already markdown).
export function ExternalConfiguration() {
  return (
    <MainContainer>
      <ConfigurationFormContainer>
        <MaxTokenNumberFormField
          initialValue={512}
          sliderTestId="ds-settings-parser-recommended-chunk-size-slider"
          numberInputTestId="ds-settings-parser-recommended-chunk-size-input"
        />
        <DelimiterFormField />
        <ChildrenDelimiterForm />
        <EnableTocToggle />
        <ImageContextWindow />
        <AutoMetadata />
        <OverlappedPercent />
      </ConfigurationFormContainer>
      <ConfigurationFormContainer>
        <AutoKeywordsFormField />
        <AutoQuestionsFormField />
      </ConfigurationFormContainer>
    </MainContainer>
  );
}
