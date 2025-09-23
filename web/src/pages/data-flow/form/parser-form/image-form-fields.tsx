import { buildOptions } from '@/utils/form';
import { isEmpty } from 'lodash';
import { useEffect, useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { ImageParseMethod } from '../../constant';
import { LanguageFormField, ParserMethodFormField } from './common-form-fields';
import { CommonProps } from './interface';
import { useSetInitialLanguage } from './use-set-initial-language';
import { buildFieldNameWithPrefix } from './utils';

const options = buildOptions(ImageParseMethod);

export function ImageFormFields({ prefix }: CommonProps) {
  const form = useFormContext();
  const parseMethodName = buildFieldNameWithPrefix('parse_method', prefix);

  const parseMethod = useWatch({
    name: parseMethodName,
  });

  const languageShown = useMemo(() => {
    return !isEmpty(parseMethod) && parseMethod !== ImageParseMethod.OCR;
  }, [parseMethod]);

  useEffect(() => {
    if (isEmpty(form.getValues(parseMethodName))) {
      form.setValue(parseMethodName, ImageParseMethod.OCR, {
        shouldValidate: true,
        shouldDirty: true,
      });
    }
  }, [form, parseMethodName]);

  useSetInitialLanguage({ prefix, languageShown });

  return (
    <>
      <ParserMethodFormField
        prefix={prefix}
        optionsWithoutLLM={options}
      ></ParserMethodFormField>
      {languageShown && <LanguageFormField prefix={prefix}></LanguageFormField>}
    </>
  );
}
