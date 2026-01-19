import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import {
  SelectWithSearch,
  SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { isEmpty } from 'lodash';
import { useEffect, useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { LanguageFormField, ParserMethodFormField } from './common-form-fields';
import { CommonProps } from './interface';
import { useSetInitialLanguage } from './use-set-initial-language';
import { buildFieldNameWithPrefix } from './utils';

const tableResultTypeOptions: SelectWithSearchFlagOptionType[] = [
  { label: 'Markdown', value: '0' },
  { label: 'HTML', value: '1' },
];

const markdownImageResponseTypeOptions: SelectWithSearchFlagOptionType[] = [
  { label: 'URL', value: '0' },
  { label: 'Text', value: '1' },
];

export function PdfFormFields({ prefix }: CommonProps) {
  const { t } = useTranslation();
  const form = useFormContext();

  const parseMethodName = buildFieldNameWithPrefix('parse_method', prefix);

  const parseMethod = useWatch({
    name: parseMethodName,
  });

  const languageShown = useMemo(() => {
    return (
      !isEmpty(parseMethod) &&
      parseMethod !== ParseDocumentType.DeepDOC &&
      parseMethod !== ParseDocumentType.PlainText &&
      parseMethod !== ParseDocumentType.TCADPParser
    );
  }, [parseMethod]);

  const tcadpOptionsShown = useMemo(() => {
    return (
      !isEmpty(parseMethod) && parseMethod === ParseDocumentType.TCADPParser
    );
  }, [parseMethod]);

  useSetInitialLanguage({ prefix, languageShown });

  useEffect(() => {
    if (isEmpty(form.getValues(parseMethodName))) {
      form.setValue(parseMethodName, ParseDocumentType.DeepDOC, {
        shouldValidate: true,
        shouldDirty: true,
      });
    }
  }, [form, parseMethodName]);

  // Set default values for TCADP options when TCADP is selected
  useEffect(() => {
    if (tcadpOptionsShown) {
      const tableResultTypeName = buildFieldNameWithPrefix(
        'table_result_type',
        prefix,
      );
      const markdownImageResponseTypeName = buildFieldNameWithPrefix(
        'markdown_image_response_type',
        prefix,
      );

      if (isEmpty(form.getValues(tableResultTypeName))) {
        form.setValue(tableResultTypeName, '1', {
          shouldValidate: true,
          shouldDirty: true,
        });
      }
      if (isEmpty(form.getValues(markdownImageResponseTypeName))) {
        form.setValue(markdownImageResponseTypeName, '1', {
          shouldValidate: true,
          shouldDirty: true,
        });
      }
    }
  }, [tcadpOptionsShown, form, prefix]);

  return (
    <>
      <ParserMethodFormField prefix={prefix}></ParserMethodFormField>
      {languageShown && <LanguageFormField prefix={prefix}></LanguageFormField>}
      {tcadpOptionsShown && (
        <>
          <RAGFlowFormItem
            name={buildFieldNameWithPrefix('table_result_type', prefix)}
            label={t('flow.tableResultType') || '表格返回形式'}
          >
            {(field) => (
              <SelectWithSearch
                value={field.value}
                onChange={field.onChange}
                options={tableResultTypeOptions}
              ></SelectWithSearch>
            )}
          </RAGFlowFormItem>
          <RAGFlowFormItem
            name={buildFieldNameWithPrefix(
              'markdown_image_response_type',
              prefix,
            )}
            label={t('flow.markdownImageResponseType') || '图片返回形式'}
          >
            {(field) => (
              <SelectWithSearch
                value={field.value}
                onChange={field.onChange}
                options={markdownImageResponseTypeOptions}
              ></SelectWithSearch>
            )}
          </RAGFlowFormItem>
        </>
      )}
    </>
  );
}
