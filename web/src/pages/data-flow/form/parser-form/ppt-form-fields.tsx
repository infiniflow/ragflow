import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import { isEmpty } from 'lodash';
import { useEffect } from 'react';
import { useFormContext } from 'react-hook-form';
import { ParserMethodFormField } from './common-form-fields';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

export function PptFormFields({ prefix }: CommonProps) {
  const form = useFormContext();

  const parseMethodName = buildFieldNameWithPrefix('parse_method', prefix);

  // PPT only supports DeepDOC and TCADPParser
  const optionsWithoutLLM = [
    { label: ParseDocumentType.DeepDOC, value: ParseDocumentType.DeepDOC },
    {
      label: ParseDocumentType.TCADPParser,
      value: ParseDocumentType.TCADPParser,
    },
  ];

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
      <ParserMethodFormField
        prefix={prefix}
        optionsWithoutLLM={optionsWithoutLLM}
      ></ParserMethodFormField>
    </>
  );
}
