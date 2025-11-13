import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import { isEmpty } from 'lodash';
import { useEffect, useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { LanguageFormField, ParserMethodFormField } from './common-form-fields';
import { CommonProps } from './interface';
import { useSetInitialLanguage } from './use-set-initial-language';
import { buildFieldNameWithPrefix } from './utils';

export function PdfFormFields({ prefix }: CommonProps) {
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

  useSetInitialLanguage({ prefix, languageShown });

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
      {languageShown && <LanguageFormField prefix={prefix}></LanguageFormField>}
    </>
  );
}
