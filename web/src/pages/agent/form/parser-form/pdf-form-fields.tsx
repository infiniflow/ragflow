import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { useWatch } from 'react-hook-form';
import {
  LanguageFormField,
  ParserMethodFormField,
  RemoveHeaderFooterFormField,
  RmdirFormField,
  TcadpFormFields,
  TwoColumnCheckFormField,
} from './common-form-fields';
import { CommonProps } from './interface';
import { DynamicPageRange } from './dynamic-page-range';
import { useSetInitialLanguage } from './use-set-initial-language';
import { buildFieldNameWithPrefix } from './utils';

export function PdfFormFields({ prefix }: CommonProps) {
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

  return (
    <>
      <TwoColumnCheckFormField prefix={prefix} />
      <RmdirFormField prefix={prefix} />
      <RemoveHeaderFooterFormField prefix={prefix} />
      <ParserMethodFormField prefix={prefix}></ParserMethodFormField>
      <DynamicPageRange prefix={prefix} />

      {languageShown && <LanguageFormField prefix={prefix}></LanguageFormField>}
      <TcadpFormFields prefix={prefix} />
    </>
  );
}
