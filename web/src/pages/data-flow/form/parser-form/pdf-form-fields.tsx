import { crossLanguageOptions } from '@/components/cross-language-form-field';
import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { isEmpty } from 'lodash';
import { useEffect, useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { ParserMethodFormField } from './common-form-fields';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

export function PdfFormFields({ prefix }: CommonProps) {
  const { t } = useTranslation();
  const form = useFormContext();

  const parseMethodName = buildFieldNameWithPrefix('parse_method', prefix);

  const parseMethod = useWatch({
    name: parseMethodName,
  });
  const lang = form.getValues(buildFieldNameWithPrefix('lang', prefix));

  const languageShown = useMemo(() => {
    return (
      !isEmpty(parseMethod) &&
      parseMethod !== ParseDocumentType.DeepDOC &&
      parseMethod !== ParseDocumentType.PlainText
    );
  }, [parseMethod]);

  useEffect(() => {
    if (languageShown && isEmpty(lang)) {
      form.setValue(
        buildFieldNameWithPrefix('lang', prefix),
        crossLanguageOptions[0].value,
        {
          shouldValidate: true,
          shouldDirty: true,
        },
      );
    }
  }, [form, lang, languageShown, prefix]);

  useEffect(() => {
    if (isEmpty(form.getValues(parseMethodName))) {
      form.setValue(parseMethodName, ParseDocumentType.DeepDOC, {
        shouldValidate: true,
        shouldDirty: true,
      });
    }
  }, [form, parseMethodName]);

  return (
    <>
      <ParserMethodFormField prefix={prefix}></ParserMethodFormField>
      {languageShown && (
        <RAGFlowFormItem
          name={buildFieldNameWithPrefix(`lang`, prefix)}
          label={t('dataflow.lang')}
        >
          {(field) => (
            <SelectWithSearch
              options={crossLanguageOptions}
              value={field.value}
              onChange={field.onChange}
            ></SelectWithSearch>
          )}
        </RAGFlowFormItem>
      )}
    </>
  );
}
