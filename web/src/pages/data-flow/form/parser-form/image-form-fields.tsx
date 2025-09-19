import { buildOptions } from '@/utils/form';
import { isEmpty } from 'lodash';
import { useEffect } from 'react';
import { useFormContext } from 'react-hook-form';
import { ImageParseMethod } from '../../constant';
import { ParserMethodFormField } from './common-form-fields';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

const options = buildOptions(ImageParseMethod);

export function ImageFormFields({ prefix }: CommonProps) {
  const form = useFormContext();
  const parseMethodName = buildFieldNameWithPrefix('parse_method', prefix);

  useEffect(() => {
    if (isEmpty(form.getValues(parseMethodName))) {
      form.setValue(parseMethodName, ImageParseMethod.OCR, {
        shouldValidate: true,
        shouldDirty: true,
      });
    }
  }, [form, parseMethodName]);

  return (
    <>
      <ParserMethodFormField
        prefix={prefix}
        optionsWithoutLLM={options}
      ></ParserMethodFormField>
    </>
  );
}
